package sim

import (
	"github.com/gofiber/fiber/v3"

	"github.com/momobasehq/mockpay/internal/store"
)

func RegisterRoutes(r fiber.Router, mtnStore *store.MTNStore, airtelStore *store.AirtelStore) {
	r.Get("/state/", getState(mtnStore, airtelStore))
	r.Post("/config/", updateConfig)
	r.Delete("/reset/", resetState(mtnStore, airtelStore))
	r.Get("/ready/", checkHealth)
}

// GET /admin/state — dump all in-memory transactions across both providers
func getState(mtnStore *store.MTNStore, airtelStore *store.AirtelStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		fr, minMs, maxMs := Global.Snapshot()
		return c.JSON(fiber.Map{
			"sim": fiber.Map{
				"failureRate": fr,
				"minDelayMs":  minMs,
				"maxDelayMs":  maxMs,
			},
			"mtn":             mtnStore.Dump(),
			"airtel":          airtelStore.Dump(),
			"pendingWebhooks": PendingWebhooks(),
		})
	}
}

// POST /admin/config — tune simulation behaviour at runtime
//
// Body: { "failureRate": 0.5, "minDelayMs": 100, "maxDelayMs": 2000 }
func updateConfig(c fiber.Ctx) error {
	var body struct {
		FailureRate *float64 `json:"failureRate"`
		MinDelayMs  *int     `json:"minDelayMs"`
		MaxDelayMs  *int     `json:"maxDelayMs"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	curRate, curMin, curMax := Global.Snapshot()
	if body.FailureRate != nil {
		curRate = *body.FailureRate
	}
	if body.MinDelayMs != nil {
		curMin = *body.MinDelayMs
	}
	if body.MaxDelayMs != nil {
		curMax = *body.MaxDelayMs
	}
	Global.Update(curRate, curMin, curMax)
	rate, minMs, maxMs := Global.Snapshot()
	return c.JSON(fiber.Map{
		"message":     "Simulation config updated",
		"failureRate": rate,
		"minDelayMs":  minMs,
		"maxDelayMs":  maxMs,
	})
}

// DELETE /admin/reset — wipe all transactions and tokens (API users are preserved)
func resetState(mtn *store.MTNStore, airtel *store.AirtelStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		mtn.Reset()
		airtel.Reset()
		ResetPendingWebhooks()
		return c.JSON(fiber.Map{"message": "All transactions, tokens, and pending webhooks cleared"})
	}
}

// GET /admin/ready — simple healthcheck
func checkHealth(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}
