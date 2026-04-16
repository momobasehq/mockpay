// Package airtel implements mock endpoints that mirror the Airtel Africa Money API.
//
// Route layout (all under /airtel):
//
//	Auth
//	  POST /auth/oauth2/token                    – client_credentials grant
//
//	Collections
//	  POST /merchant/v2/payments/                – initiate payment (collect from subscriber)
//	  GET  /standard/v1/payments/:id             – query payment status
//
//	Disbursements
//	  POST /standard/v1/disbursements/           – initiate disbursement (pay to subscriber)
//	  GET  /standard/v1/disbursements/:id        – query disbursement status
//
//	Refunds
//	  POST /standard/v1/payments/refund          – refund a payment
package airtel

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/momobasehq/mockpay/internal/sim"
	"github.com/momobasehq/mockpay/internal/store"
)

// RegisterRoutes attaches all Airtel mock routes to r.
func RegisterRoutes(r fiber.Router, s *store.AirtelStore) {
	// Auth
	r.Post("/auth/oauth2/token", getToken(s))

	// Collections
	r.Post("/merchant/v2/payments/", airtelAuth(s), initiatePayment(s))
	r.Get("/standard/v1/payments/:id", airtelAuth(s), getPayment(s))

	// Disbursements
	r.Post("/standard/v1/disbursements/", airtelAuth(s), initiateDisbursement(s))
	r.Get("/standard/v1/disbursements/:id", airtelAuth(s), getDisbursement(s))

	// Refunds
	r.Post("/standard/v1/payments/refund", airtelAuth(s), refundPayment(s))
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

func airtelAuth(s *store.AirtelStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := c.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(
				airtelErr("ESB000001", "401", "Unauthorized request"),
			)
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if !s.ValidateToken(token) {
			return c.Status(fiber.StatusUnauthorized).JSON(
				airtelErr("ESB000001", "401", "Token is invalid or has expired"),
			)
		}
		return c.Next()
	}
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

// POST /airtel/auth/oauth2/token
func getToken(s *store.AirtelStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		var body struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			GrantType    string `json:"grant_type"`
		}
		// Tolerate both JSON and form-encoded bodies.
		_ = c.Bind().Body(&body)

		// In sandbox mode we accept any credentials.
		token := uuid.NewString() + uuid.NewString()
		s.StoreToken(&store.AirtelTokenRecord{
			AccessToken: token,
			ExpiresAt:   time.Now().Add(3600 * time.Second),
			ClientID:    body.ClientID,
		})
		return c.JSON(fiber.Map{
			"access_token": token,
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}
}

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

// POST /airtel/merchant/v2/payments/
func initiatePayment(s *store.AirtelStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		var body struct {
			Reference  string `json:"reference"`
			Subscriber struct {
				Country  string `json:"country"`
				Currency string `json:"currency"`
				MSISDN   string `json:"msisdn"`
			} `json:"subscriber"`
			Transaction struct {
				Amount   interface{} `json:"amount"` // API accepts both number and string
				Country  string      `json:"country"`
				Currency string      `json:"currency"`
				ID       string      `json:"id"`
			} `json:"transaction"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				airtelErr("ESB000002", "400", "Invalid request body"),
			)
		}
		if body.Subscriber.MSISDN == "" || body.Transaction.Amount == nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				airtelErr("ESB000009", "400", "subscriber.msisdn and transaction.amount are required"),
			)
		}

		txID := body.Transaction.ID
		if txID == "" {
			txID = uuid.NewString()
		}
		if _, exists := s.GetPayment(txID); exists {
			return c.Status(fiber.StatusConflict).JSON(
				airtelErr("ESB000026", "409", "Duplicate transaction ID"),
			)
		}

		callbackURL := c.Get("X-Callback-Url")
		tx := &store.AirtelTransaction{
			ID:          txID,
			Reference:   body.Reference,
			Amount:      fmt.Sprintf("%v", body.Transaction.Amount),
			Currency:    body.Transaction.Currency,
			Country:     body.Transaction.Country,
			MSISDN:      body.Subscriber.MSISDN,
			Status:      store.StatusPending,
			Message:     "Waiting for customer confirmation",
			CallbackURL: callbackURL,
			CreatedAt:   time.Now(),
		}
		s.SavePayment(tx)

		force := c.Query("force")
		go processPaymentAsync(s, txID, callbackURL, force)

		return c.JSON(airtelOK(fiber.Map{
			"transaction": fiber.Map{
				"id":      txID,
				"status":  airtelPending,
				"message": "Waiting for customer confirmation",
			},
		}))
	}
}

func processPaymentAsync(s *store.AirtelStore, txID, callbackURL, force string) {
	time.Sleep(sim.Global.Delay())

	var status store.TransactionStatus
	var airtelMoneyID, message string

	if sim.Global.ShouldFail(force) {
		status = store.StatusFailed
		message = "Transaction failed. Insufficient funds or subscriber not reachable."
	} else {
		status = store.StatusSuccessful
		airtelMoneyID = "CI" + strings.ToUpper(uuid.NewString()[:10])
		message = "Transaction Successful"
	}
	s.UpdatePaymentStatus(txID, status, airtelMoneyID, message)

	if callbackURL != "" {
		tx, _ := s.GetPayment(txID)
		sim.FireWebhook(callbackURL, airtelCallbackPayload(tx))
	}
}

// GET /airtel/standard/v1/payments/:id
func getPayment(s *store.AirtelStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("id")
		tx, exists := s.GetPayment(id)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(
				airtelErr("ESB000033", "404", "Transaction not found"),
			)
		}
		return c.JSON(airtelOK(fiber.Map{
			"transaction": fiber.Map{
				"id":              tx.ID,
				"airtel_money_id": tx.AirtelMoneyID,
				"status":          airtelStatusCode(tx.Status),
				"message":         tx.Message,
			},
		}))
	}
}

// ---------------------------------------------------------------------------
// Disbursements
// ---------------------------------------------------------------------------

// POST /airtel/standard/v1/disbursements/
func initiateDisbursement(s *store.AirtelStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		var body struct {
			Payee struct {
				MSISDN string `json:"msisdn"`
			} `json:"payee"`
			Reference   string `json:"reference"`
			PIN         string `json:"pin"`
			Transaction struct {
				Amount   interface{} `json:"amount"`
				Currency string      `json:"currency"`
				ID       string      `json:"id"`
			} `json:"transaction"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				airtelErr("ESB000002", "400", "Invalid request body"),
			)
		}
		if body.Payee.MSISDN == "" || body.Transaction.Amount == nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				airtelErr("ESB000009", "400", "payee.msisdn and transaction.amount are required"),
			)
		}

		txID := body.Transaction.ID
		if txID == "" {
			txID = uuid.NewString()
		}
		if _, exists := s.GetDisbursement(txID); exists {
			return c.Status(fiber.StatusConflict).JSON(
				airtelErr("ESB000026", "409", "Duplicate transaction ID"),
			)
		}

		callbackURL := c.Get("X-Callback-Url")
		tx := &store.AirtelTransaction{
			ID:          txID,
			Reference:   body.Reference,
			Amount:      fmt.Sprintf("%v", body.Transaction.Amount),
			Currency:    body.Transaction.Currency,
			MSISDN:      body.Payee.MSISDN,
			Status:      store.StatusPending,
			Message:     "Disbursement in progress",
			CallbackURL: callbackURL,
			CreatedAt:   time.Now(),
		}
		s.SaveDisbursement(tx)

		force := c.Query("force")
		go processDisbursementAsync(s, txID, callbackURL, force)

		return c.JSON(airtelOK(fiber.Map{
			"transaction": fiber.Map{
				"id":      txID,
				"status":  airtelPending,
				"message": "Disbursement in progress",
			},
		}))
	}
}

func processDisbursementAsync(s *store.AirtelStore, txID, callbackURL, force string) {
	time.Sleep(sim.Global.Delay())

	var status store.TransactionStatus
	var airtelMoneyID, message string

	if sim.Global.ShouldFail(force) {
		status = store.StatusFailed
		message = "Disbursement failed. Recipient not found or account inactive."
	} else {
		status = store.StatusSuccessful
		airtelMoneyID = "DI" + strings.ToUpper(uuid.NewString()[:10])
		message = "Disbursement Successful"
	}
	s.UpdateDisbursementStatus(txID, status, airtelMoneyID, message)

	if callbackURL != "" {
		tx, _ := s.GetDisbursement(txID)
		sim.FireWebhook(callbackURL, airtelCallbackPayload(tx))
	}
}

// GET /airtel/standard/v1/disbursements/:id
func getDisbursement(s *store.AirtelStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("id")
		tx, exists := s.GetDisbursement(id)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(
				airtelErr("ESB000033", "404", "Transaction not found"),
			)
		}
		return c.JSON(airtelOK(fiber.Map{
			"transaction": fiber.Map{
				"id":              tx.ID,
				"airtel_money_id": tx.AirtelMoneyID,
				"status":          airtelStatusCode(tx.Status),
				"message":         tx.Message,
			},
		}))
	}
}

// ---------------------------------------------------------------------------
// Refunds
// ---------------------------------------------------------------------------

// POST /airtel/standard/v1/payments/refund
func refundPayment(s *store.AirtelStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		var body struct {
			Transaction struct {
				AirtelMoneyID string `json:"airtel_money_id"`
			} `json:"transaction"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				airtelErr("ESB000002", "400", "Invalid request body"),
			)
		}
		if body.Transaction.AirtelMoneyID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(
				airtelErr("ESB000009", "400", "transaction.airtel_money_id is required"),
			)
		}

		refundID := "RF" + strings.ToUpper(uuid.NewString()[:10])
		// Store as a refund record for inspection.
		s.SaveRefund(&store.AirtelTransaction{
			ID:            refundID,
			AirtelMoneyID: body.Transaction.AirtelMoneyID,
			Status:        store.StatusSuccessful,
			Message:       "Refund Successful",
			CreatedAt:     time.Now(),
		})

		return c.JSON(airtelOK(fiber.Map{
			"transaction": fiber.Map{
				"airtel_money_id": body.Transaction.AirtelMoneyID,
				"status":          airtelSuccess,
				"message":         "Refund Successful",
			},
		}))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// Airtel transaction status codes used in API responses.
const (
	airtelPending = "DP" // Debit Pending
	airtelSuccess = "TS" // Transaction Successful
	airtelFailed  = "TF" // Transaction Failed
)

func airtelStatusCode(s store.TransactionStatus) string {
	switch s {
	case store.StatusSuccessful:
		return airtelSuccess
	case store.StatusFailed:
		return airtelFailed
	default:
		return airtelPending
	}
}

// airtelOK wraps data in the standard Airtel success envelope.
func airtelOK(data interface{}) fiber.Map {
	return fiber.Map{
		"data": data,
		"status": fiber.Map{
			"code":        "200",
			"message":     "SUCCESS",
			"result_code": "ESB000010",
			"success":     true,
		},
	}
}

// airtelErr wraps an error in the standard Airtel error envelope.
func airtelErr(resultCode, httpCode, message string) fiber.Map {
	return fiber.Map{
		"data": nil,
		"status": fiber.Map{
			"code":        httpCode,
			"message":     message,
			"result_code": resultCode,
			"success":     false,
		},
	}
}

// airtelCallbackPayload is the webhook body sent to the caller's callback URL.
func airtelCallbackPayload(tx *store.AirtelTransaction) fiber.Map {
	return fiber.Map{
		"transaction": fiber.Map{
			"id":              tx.ID,
			"airtel_money_id": tx.AirtelMoneyID,
			"msisdn":          tx.MSISDN,
			"amount":          tx.Amount,
			"currency":        tx.Currency,
			"status":          airtelStatusCode(tx.Status),
			"message":         tx.Message,
		},
	}
}
