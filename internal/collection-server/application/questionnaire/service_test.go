package questionnaire

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	questionnairepb "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/fangcun-mount/qs-server/internal/collection-server/domain/questionnaire"
)

// MockQuestionnaireClient 模拟问卷客户端
type MockQuestionnaireClient struct {
	mock.Mock
}

func (m *MockQuestionnaireClient) GetQuestionnaire(ctx context.Context, code string) (*questionnairepb.GetQuestionnaireResponse, error) {
	args := m.Called(ctx, code)
	return args.Get(0).(*questionnairepb.GetQuestionnaireResponse), args.Error(1)
}

func (m *MockQuestionnaireClient) ListQuestionnaires(ctx context.Context, req *questionnairepb.ListQuestionnairesRequest) (*questionnairepb.ListQuestionnairesResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*questionnairepb.ListQuestionnairesResponse), args.Error(1)
}

func (m *MockQuestionnaireClient) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockQuestionnaireClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestNewService(t *testing.T) {
	mockClient := &MockQuestionnaireClient{}
	service := NewService(mockClient)

	assert.NotNil(t, service)
}

func TestService_GetQuestionnaire(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		setupMock      func(*MockQuestionnaireClient)
		expectedError  bool
		expectedResult *questionnaire.Questionnaire
	}{
		{
			name: "successful get questionnaire",
			code: "test-questionnaire",
			setupMock: func(mockClient *MockQuestionnaireClient) {
				protoQuestionnaire := &questionnairepb.Questionnaire{
					Code:        "test-questionnaire",
					Title:       "Test Questionnaire",
					Description: "A test questionnaire",
					Status:      "published",
					CreatedAt:   "2023-01-01T00:00:00Z",
					UpdatedAt:   "2023-01-01T00:00:00Z",
					Questions: []*questionnairepb.Question{
						{
							Code:  "q1",
							Title: "Question 1",
							Type:  "text",
						},
					},
				}

				mockClient.On("GetQuestionnaire", mock.Anything, "test-questionnaire").
					Return(&questionnairepb.GetQuestionnaireResponse{
						Questionnaire: protoQuestionnaire,
					}, nil)
			},
			expectedError: false,
			expectedResult: &questionnaire.Questionnaire{
				Code:        "test-questionnaire",
				Title:       "Test Questionnaire",
				Description: "A test questionnaire",
				Status:      "published",
			},
		},
		{
			name: "empty code",
			code: "",
			setupMock: func(mockClient *MockQuestionnaireClient) {
				// 不需要设置 mock，因为会在验证阶段失败
			},
			expectedError:  true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockQuestionnaireClient{}
			if tt.setupMock != nil {
				tt.setupMock(mockClient)
			}

			service := NewService(mockClient)
			result, err := service.GetQuestionnaire(context.Background(), tt.code)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.Code, result.Code)
				assert.Equal(t, tt.expectedResult.Title, result.Title)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestService_ValidateQuestionnaireCode(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		setupMock     func(*MockQuestionnaireClient)
		expectedError bool
	}{
		{
			name: "valid questionnaire code",
			code: "valid-questionnaire",
			setupMock: func(mockClient *MockQuestionnaireClient) {
				protoQuestionnaire := &questionnairepb.Questionnaire{
					Code:   "valid-questionnaire",
					Title:  "Valid Questionnaire",
					Status: "published",
					Questions: []*questionnairepb.Question{
						{
							Code:  "q1",
							Title: "Question 1",
							Type:  "text",
						},
					},
				}

				mockClient.On("GetQuestionnaire", mock.Anything, "valid-questionnaire").
					Return(&questionnairepb.GetQuestionnaireResponse{
						Questionnaire: protoQuestionnaire,
					}, nil)
			},
			expectedError: false,
		},
		{
			name: "empty code",
			code: "",
			setupMock: func(mockClient *MockQuestionnaireClient) {
				// 不需要设置 mock
			},
			expectedError: true,
		},
		{
			name: "invalid code format",
			code: "invalid@code",
			setupMock: func(mockClient *MockQuestionnaireClient) {
				// 不需要设置 mock
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockQuestionnaireClient{}
			if tt.setupMock != nil {
				tt.setupMock(mockClient)
			}

			service := NewService(mockClient)
			err := service.ValidateQuestionnaireCode(context.Background(), tt.code)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}
