// Package testeeaccess owns the collection-server User -> Testee access check.
package testeeaccess

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	testeeapp "github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrAccessDenied      = errors.New("testee access denied")
	ErrAccessUnavailable = errors.New("testee access authorization unavailable")
)

type TesteeReader interface {
	GetTestee(ctx context.Context, testeeID uint64) (*testeeapp.TesteeResponse, error)
}

type ProfileLinkChecker interface {
	IsEnabled() bool
	HasActiveProfileLink(ctx context.Context, userID, profileID string) (bool, error)
}

// Authorizer proves that an IAM User can represent a Testee.
type Authorizer struct {
	testees TesteeReader
	links   ProfileLinkChecker
}

func NewAuthorizer(testees TesteeReader, links ProfileLinkChecker) *Authorizer {
	return &Authorizer{testees: testees, links: links}
}

func (a *Authorizer) Authorize(ctx context.Context, userID string, testeeID uint64) error {
	started := time.Now()
	result := "error"
	defer func() {
		observeTesteeAccess(result, time.Since(started))
	}()

	userID = strings.TrimSpace(userID)
	if userID == "" || testeeID == 0 {
		result = "denied"
		return ErrAccessDenied
	}
	if a == nil || isNilDependency(a.testees) || isNilDependency(a.links) || !a.links.IsEnabled() {
		result = "misconfigured"
		return ErrAccessUnavailable
	}

	testee, err := a.testees.GetTestee(ctx, testeeID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			result = "denied"
			return ErrAccessDenied
		}
		logger.L(ctx).Errorw("report status testee lookup failed",
			"action", "report_status_testee_access",
			"user_id", userID,
			"testee_id", testeeID,
			"result", "error",
			"error", err.Error(),
		)
		return fmt.Errorf("%w: testee lookup failed", ErrAccessUnavailable)
	}
	if testee == nil || strings.TrimSpace(testee.IAMProfileID) == "" {
		result = "denied"
		logger.L(ctx).Warnw("report status testee access denied",
			"action", "report_status_testee_access",
			"user_id", userID,
			"testee_id", testeeID,
			"result", "denied",
			"reason", "profile_unavailable",
		)
		return ErrAccessDenied
	}

	allowed, err := a.links.HasActiveProfileLink(ctx, userID, testee.IAMProfileID)
	if err != nil {
		logger.L(ctx).Errorw("report status profile link check failed",
			"action", "report_status_testee_access",
			"user_id", userID,
			"testee_id", testeeID,
			"result", "error",
			"error", err.Error(),
		)
		return fmt.Errorf("%w: profile link check failed", ErrAccessUnavailable)
	}
	if !allowed {
		result = "denied"
		logger.L(ctx).Warnw("report status testee access denied",
			"action", "report_status_testee_access",
			"user_id", userID,
			"testee_id", testeeID,
			"result", "denied",
			"reason", "profile_link",
		)
		return ErrAccessDenied
	}

	result = "allowed"
	return nil
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
