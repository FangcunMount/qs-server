package plansubmit

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

const (
	seedPlanRecoverableRetries = 3
	seedPlanRecoverableMinWait = 30 * time.Second
	seedPlanRecoverableMaxWait = 120 * time.Second
)

func runSeedPlanOperationWithRecovery(
	ctx context.Context,
	logger interface{ Warnw(string, ...interface{}) },
	verbose bool,
	operation string,
	resourceID string,
	fn func() error,
) error {
	if fn == nil {
		return fmt.Errorf("seed plan operation %s is nil", operation)
	}

	var lastErr error
	for attempt := 0; attempt <= seedPlanRecoverableRetries; attempt++ {
		if attempt > 0 {
			delay := seedPlanRecoverableDelay()
			if verbose {
				logger.Warnw("Seed plan recoverable error, waiting before retry",
					"operation", operation,
					"resource_id", resourceID,
					"attempt", attempt,
					"max_attempts", seedPlanRecoverableRetries,
					"delay_seconds", int(delay.Seconds()),
					"error", lastErr.Error(),
				)
			}
			if err := sleepWithContext(ctx, delay); err != nil {
				return err
			}
		}

		if err := fn(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			if !isSeedPlanRecoverableError(err) || attempt == seedPlanRecoverableRetries {
				return err
			}
			continue
		}
		return nil
	}

	return lastErr
}

func isSeedPlanRecoverableError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	recoverablePatterns := []string{
		"context deadline exceeded",
		"client.timeout exceeded",
		"http_status=500",
		"http_status=502",
		"http_status=503",
		"http_status=504",
		"http error: status=500",
		"http error: status=502",
		"http error: status=503",
		"http error: status=504",
		"connection reset by peer",
		"broken pipe",
		"tls handshake timeout",
		"timeout awaiting headers",
		"i/o timeout",
	}
	for _, pattern := range recoverablePatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

func seedPlanRecoverableDelay() time.Duration {
	if seedPlanRecoverableMaxWait <= seedPlanRecoverableMinWait {
		return seedPlanRecoverableMinWait
	}
	span := seedPlanRecoverableMaxWait - seedPlanRecoverableMinWait
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return seedPlanRecoverableMinWait + time.Duration(rng.Int63n(int64(span)+1))
}

func newPlanQuestionnaireVersionMismatchError(
	scaleCode string,
	questionnaireCode string,
	scaleQuestionnaireVersion string,
	loadedQuestionnaireVersion string,
) error {
	normalizedScaleCode := strings.ToLower(strings.TrimSpace(scaleCode))
	return fmt.Errorf(
		"questionnaire version mismatch for plan task submit: scale_code=%s questionnaire_code=%s scale_questionnaire_version=%s loaded_questionnaire_version=%s; seeddata loads questionnaire detail by code only, so this usually means the scale still comes from apiserver Redis cache or the scale is bound to a different questionnaire version; if you changed scale.questionnaire_version directly in MongoDB, delete Redis key scale:%s (or <cache.namespace>:scale:%s) and retry",
		scaleCode,
		questionnaireCode,
		scaleQuestionnaireVersion,
		loadedQuestionnaireVersion,
		normalizedScaleCode,
		normalizedScaleCode,
	)
}
