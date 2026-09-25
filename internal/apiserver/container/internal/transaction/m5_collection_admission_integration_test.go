//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	collectionanswersheet "github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	collectionquestionnaire "github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	collectiongrpc "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	collectionacl "github.com/FangcunMount/qs-server/internal/collection-server/port/acl"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type m5ReceiptQuestionnaireRepo struct {
	domainquestionnaire.Repository
	value *domainquestionnaire.Questionnaire
}

func (r m5ReceiptQuestionnaireRepo) FindByCodeVersion(_ context.Context, code, version string) (*domainquestionnaire.Questionnaire, error) {
	if code == r.value.GetCode().Value() && version == r.value.GetVersion().Value() {
		return r.value, nil
	}
	return nil, nil
}

type m5ReceiptBinding struct {
	rulesetport.AssessmentBindingResolver
}

func (m5ReceiptBinding) ResolveAssessmentBinding(context.Context, string, string) (rulesetport.AssessmentBinding, bool, error) {
	return rulesetport.AssessmentBinding{}, false, nil
}

type m5ReceiptActor struct{}

func (m5ReceiptActor) GetTestee(context.Context, uint64) (*collectionanswersheet.ActorTestee, error) {
	return &collectionanswersheet.ActorTestee{OrgID: 501, IAMProfileID: "test-profile"}, nil
}
func (m5ReceiptActor) TesteeExists(context.Context, uint64, uint64) (bool, uint64, error) {
	return false, 0, nil
}

type m5ReceiptProfileLink struct{}

func (m5ReceiptProfileLink) IsEnabled() bool         { return true }
func (m5ReceiptProfileLink) GetDefaultOrgID() uint64 { return 501 }
func (m5ReceiptProfileLink) HasActiveProfileLink(context.Context, string, string) (bool, error) {
	return true, nil
}

type m5ReceiptQuestionnaireReader struct{}

func (m5ReceiptQuestionnaireReader) Get(context.Context, string, string) (*collectionquestionnaire.QuestionnaireResponse, error) {
	return &collectionquestionnaire.QuestionnaireResponse{
		Code: "QNR-M5", Version: "1.0.0", Status: "published",
		Questions: []collectionquestionnaire.QuestionResponse{{Code: "Q1", Type: "Text"}},
	}, nil
}

type m5ReceiptStager struct {
	inner *mongostandard.Stager
	fail  atomic.Bool
}

func (s *m5ReceiptStager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	if err := s.inner.Stage(ctx, events...); err != nil {
		return err
	}
	if s.fail.Load() {
		return errors.New("injected failure after standard Outbox staging")
	}
	return nil
}

// A Collection acceptance response must follow a committed AnswerSheet and
// standard Outbox intent, including when an earlier attempt rolled back.
func TestM5CollectionAdmissionReceiptFollowsStandardMongoCommit(t *testing.T) {
	uri := os.Getenv("RM_QS_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") {
		t.Fatal("disposable rm-test replica-set URI required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	defer client.Disconnect(context.Background())
	db := client.Database("rm_qs_collection_admission_m5")
	defer db.Drop(context.Background())
	require.NoError(t, db.CreateCollection(ctx, "rm_outbox"))
	outbox := db.Collection("rm_outbox")
	_, err = outbox.Indexes().CreateMany(ctx, sdkmongo.Indexes())
	require.NoError(t, err)
	sheets, err := mongoanswersheet.NewRepository(db)
	require.NoError(t, err)
	catalog, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  answersheet.submitted:
    topic: evaluation
    delivery: durable_outbox
    aggregate: AnswerSheet
    domain: survey
    handler: answersheet_submitted_handler
`))
	require.NoError(t, err)
	inner, err := mongostandard.NewStager(outbox, eventcatalog.NewCatalog(catalog), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	stager := &m5ReceiptStager{inner: inner}
	stager.fail.Store(true)
	durable := appanswersheet.NewTransactionalSubmissionDurableStore(
		NewMongoRunner(db, MongoRunnerOptions{Boundary: "m5_collection_admission", Limiter: &transactionLimiterSpy{}}),
		sheets, stager, standardoutbox.NewPostCommitWake(),
	)
	questionnaire, err := domainquestionnaire.NewQuestionnaire(meta.NewCode("QNR-M5"), "Independent questionnaire",
		domainquestionnaire.WithVersion("1.0.0"), domainquestionnaire.WithStatus(domainquestionnaire.STATUS_PUBLISHED))
	require.NoError(t, err)
	question, err := domainquestionnaire.NewQuestion(
		domainquestionnaire.WithCode(meta.NewCode("Q1")), domainquestionnaire.WithStem("Question 1"),
		domainquestionnaire.WithQuestionType(domainquestionnaire.TypeText),
	)
	require.NoError(t, err)
	require.NoError(t, questionnaire.AddQuestion(question))
	apiService := appanswersheet.NewSubmissionService(sheets, durable, m5ReceiptQuestionnaireRepo{value: questionnaire}, nil)
	apiService.(appanswersheet.AssessmentBindingInjector).SetAssessmentBindingResolver(m5ReceiptBinding{})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	grpcservice.NewAnswerSheetService(apiService).RegisterService(server)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	defer func() { server.Stop(); require.NoError(t, <-serverDone) }()
	baseClient, err := collectiongrpc.NewClient(&collectiongrpc.ClientConfig{Endpoint: listener.Addr().String(), Timeout: 5 * time.Second},
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer baseClient.Close()
	answerSheetClient := collectiongrpc.NewAnswerSheetClient(baseClient)
	collectionService := collectionanswersheet.NewSubmissionService(
		collectionacl.NewAnswerSheetBFFWriter(answerSheetClient),
		collectionacl.NewAnswerSheetDurableResultReader(answerSheetClient), nil,
		m5ReceiptActor{}, m5ReceiptProfileLink{},
		nil, nil, m5ReceiptQuestionnaireReader{}, 5*time.Second,
	)
	request := &collectionanswersheet.SubmitAnswerSheetRequest{
		QuestionnaireCode: "QNR-M5", QuestionnaireVersion: "1.0.0", IdempotencyKey: "m5-collection-receipt-1",
		TesteeID: 401, Answers: []collectionanswersheet.Answer{{QuestionCode: "Q1", QuestionType: "Text", Value: `"accepted"`}},
	}
	_, err = collectionService.AcceptDurably(ctx, "request-first", 301, request)
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.EqualValues(t, 0, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{}))
	require.EqualValues(t, 0, countStandardDocs(t, ctx, outbox, bson.M{}))

	stager.fail.Store(false)
	accepted, err := collectionService.AcceptDurably(ctx, "request-retry", 301, request)
	require.NoError(t, err)
	require.NotNil(t, accepted)
	id, err := strconv.ParseUint(accepted.ID, 10, 64)
	require.NoError(t, err)
	require.NotZero(t, id)
	require.EqualValues(t, 1, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": id}))
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{"event_type": "answersheet.submitted", "state": "pending"}))
	sheet, err := sheets.FindByID(ctx, meta.FromUint64(id))
	require.NoError(t, err)
	require.NotNil(t, sheet)
	require.Equal(t, domainanswersheet.AdmissionPurposeIndependentQuestionnaire, sheet.SubmissionContext().Admission().Purpose())

	repeated, err := collectionService.AcceptDurably(ctx, "request-repeat", 301, request)
	require.NoError(t, err)
	require.Equal(t, accepted.ID, repeated.ID)
	require.EqualValues(t, 1, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{}))
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{}))
}
