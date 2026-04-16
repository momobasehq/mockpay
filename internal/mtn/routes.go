// Package mtn implements mock endpoints that mirror the MTN MoMo sandbox API.
//
// Route layout (all under /mtn):
//
//	Collections
//	  POST   /collection/token/                              – obtain access token
//	  POST   /collection/v1_0/requesttopay                  – initiate collection
//	  GET    /collection/v1_0/requesttopay/:referenceId      – query status
//	  GET    /collection/v1_0/account/balance               – account balance
//
//	Disbursements
//	  POST   /disbursement/token/                            – obtain access token
//	  POST   /disbursement/v1_0/transfer                    – initiate disbursement
//	  GET    /disbursement/v1_0/transfer/:referenceId        – query status
//	  GET    /disbursement/v1_0/account/balance             – account balance
package mtn

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/momobasehq/mockpay/internal/sim"
	"github.com/momobasehq/mockpay/internal/store"
)

// RegisterRoutes attaches all MTN mock routes to r.
func RegisterRoutes(r fiber.Router, s *store.MTNStore) {
	// Collections
	r.Post("/collection/token/", collectionToken(s))
	r.Post("/collection/v1_0/requesttopay", mtnAuth(s, "collection"), requestToPay(s))
	r.Get("/collection/v1_0/requesttopay/:referenceId", mtnAuth(s, "collection"), getRequestToPay(s))
	r.Get("/collection/v1_0/account/balance", mtnAuth(s, "collection"), collectionBalance)

	// Disbursements
	r.Post("/disbursement/token/", disbursementToken(s))
	r.Post("/disbursement/v1_0/transfer", mtnAuth(s, "disbursement"), transfer(s))
	r.Get("/disbursement/v1_0/transfer/:referenceId", mtnAuth(s, "disbursement"), getTransfer(s))
	r.Get("/disbursement/v1_0/account/balance", mtnAuth(s, "disbursement"), disbursementBalance)
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// mtnAuth validates the Bearer token and its scope before letting a request through.
func mtnAuth(s *store.MTNStore, scope string) fiber.Handler {
	return func(c fiber.Ctx) error {
		auth := c.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(mtnErr(
				"UNAUTHORIZED", "Missing or invalid Authorization header",
			))
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if !s.ValidateToken(token, scope) {
			return c.Status(fiber.StatusUnauthorized).JSON(mtnErr(
				"UNAUTHORIZED", "Token is invalid, expired, or wrong scope",
			))
		}
		return c.Next()
	}
}

// ---------------------------------------------------------------------------
// Token endpoints (Basic auth: base64(apiUser:apiKey))
// ---------------------------------------------------------------------------

func issueToken(s *store.MTNStore, scope string) fiber.Handler {
	return func(c fiber.Ctx) error {
		userID, apiKey, ok := parseBasicAuth(c)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":             "invalid_client",
				"error_description": "Authorization header must be Basic base64(userId:apiKey)",
			})
		}
		if !s.ValidateCredentials(userID, apiKey) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":             "invalid_client",
				"error_description": "Invalid API user or API key",
			})
		}
		token := uuid.NewString() + uuid.NewString()
		s.StoreToken(&store.MTNTokenRecord{
			Token:     token,
			ExpiresAt: time.Now().Add(3600 * time.Second),
			OwnerID:   userID,
			Scope:     scope,
		})
		return c.JSON(fiber.Map{
			"access_token": token,
			"token_type":   "access_token",
			"expires_in":   3600,
		})
	}
}

// POST /mtn/collection/token/
func collectionToken(s *store.MTNStore) fiber.Handler {
	return issueToken(s, "collection")
}

// POST /mtn/disbursement/token/
func disbursementToken(s *store.MTNStore) fiber.Handler {
	return issueToken(s, "disbursement")
}

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

// POST /mtn/collection/v1_0/requesttopay
func requestToPay(s *store.MTNStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		refID := c.Get("X-Reference-Id")

		if refID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(mtnErr("INVALID_REQUEST", "X-Reference-Id is required"))
		}

		callbackURL := c.Get("X-Callback-Url")
		env := envHeader(c)

		var body struct {
			Amount     string `json:"amount"`
			Currency   string `json:"currency"`
			ExternalID string `json:"externalId"`
			Payer      struct {
				PartyIDType string `json:"partyIdType"`
				PartyID     string `json:"partyId"`
			} `json:"payer"`
			PayerMessage string `json:"payerMessage"`
			PayeeNote    string `json:"payeeNote"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(mtnErr("INVALID_BODY", "Cannot parse request body"))
		}
		if body.Amount == "" || body.Currency == "" || body.Payer.PartyID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(mtnErr(
				"INVALID_REQUEST", "amount, currency and payer.partyId are required",
			))
		}
		if _, exists := s.GetCollection(refID); exists {
			return c.Status(fiber.StatusConflict).JSON(mtnErr(
				"RESOURCE_ALREADY_EXIST", "Duplicated reference id. Request already in progress",
			))
		}

		tx := &store.MTNTransaction{
			ReferenceID:  refID,
			Amount:       body.Amount,
			Currency:     body.Currency,
			ExternalID:   body.ExternalID,
			PartyIDType:  body.Payer.PartyIDType,
			PartyID:      body.Payer.PartyID,
			PayerMessage: body.PayerMessage,
			PayeeNote:    body.PayeeNote,
			Status:       store.StatusPending,
			CallbackURL:  callbackURL,
			Environment:  env,
			CreatedAt:    time.Now(),
		}
		s.SaveCollection(tx)

		force := c.Query("force") // ?force=fail or ?force=success for deterministic testing
		go processCollectionAsync(s, refID, callbackURL, force)

		return c.Status(fiber.StatusAccepted).Send(nil) // 202 – MTN spec
	}
}

func processCollectionAsync(s *store.MTNStore, refID, callbackURL, force string) {
	time.Sleep(sim.Global.Delay())

	var (
		status  store.TransactionStatus
		finTxID string
		reason  *store.MTNErrorReason
	)
	if sim.Global.ShouldFail(force) {
		if rand.Int() > rand.Int() {
			status = store.StatusCancelled
			reason = &store.MTNErrorReason{
				Code:    "PAYER_DECLINED",
				Message: "Payer account declined request",
			}
		} else {
			status = store.StatusFailed
			reason = &store.MTNErrorReason{
				Code:    "PAYER_NOT_FOUND",
				Message: "Payer account not found or inactive",
			}
		}
	} else {
		status = store.StatusSuccessful
		finTxID = "FIN" + strings.ToUpper(uuid.NewString()[:8])
	}

	s.UpdateCollectionStatus(refID, status, finTxID, reason)

	if callbackURL != "" {
		tx, _ := s.GetCollection(refID)
		sim.FireWebhook(callbackURL, mtnCallbackPayload(tx, "collection"))
	}
}

// GET /mtn/collection/v1_0/requesttopay/:referenceId
func getRequestToPay(s *store.MTNStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		refID := c.Params("referenceId")
		tx, exists := s.GetCollection(refID)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(mtnErr("RESOURCE_NOT_FOUND", "Request not found"))
		}
		return c.JSON(mtnTxResponse(tx, "payer"))
	}
}

// GET /mtn/collection/v1_0/account/balance
func collectionBalance(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"availableBalance": fmt.Sprintf("%.2f", 1_000_000+rand.Float64()*9_000_000),
		"currency":         "UGX",
	})
}

// ---------------------------------------------------------------------------
// Disbursements
// ---------------------------------------------------------------------------

// POST /mtn/disbursement/v1_0/transfer
func transfer(s *store.MTNStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		refID := c.Get("X-Reference-Id")
		if refID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(mtnErr("INVALID_REQUEST", "X-Reference-Id is required"))
		}
		callbackURL := c.Get("X-Callback-Url")
		env := envHeader(c)

		var body struct {
			Amount     string `json:"amount"`
			Currency   string `json:"currency"`
			ExternalID string `json:"externalId"`
			Payee      struct {
				PartyIDType string `json:"partyIdType"`
				PartyID     string `json:"partyId"`
			} `json:"payee"`
			PayerMessage string `json:"payerMessage"`
			PayeeNote    string `json:"payeeNote"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(mtnErr("INVALID_BODY", "Cannot parse request body"))
		}
		if body.Amount == "" || body.Currency == "" || body.Payee.PartyID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(mtnErr(
				"INVALID_REQUEST", "amount, currency and payee.partyId are required",
			))
		}
		if _, exists := s.GetDisbursement(refID); exists {
			return c.Status(fiber.StatusConflict).JSON(mtnErr(
				"RESOURCE_ALREADY_EXIST", "Duplicated reference id",
			))
		}

		tx := &store.MTNTransaction{
			ReferenceID:  refID,
			Amount:       body.Amount,
			Currency:     body.Currency,
			ExternalID:   body.ExternalID,
			PartyIDType:  body.Payee.PartyIDType,
			PartyID:      body.Payee.PartyID,
			PayerMessage: body.PayerMessage,
			PayeeNote:    body.PayeeNote,
			Status:       store.StatusPending,
			CallbackURL:  callbackURL,
			Environment:  env,
			CreatedAt:    time.Now(),
		}
		s.SaveDisbursement(tx)

		force := c.Query("force")
		go processDisbursementAsync(s, refID, callbackURL, force)

		return c.Status(fiber.StatusAccepted).Send(nil)
	}
}

func processDisbursementAsync(s *store.MTNStore, refID, callbackURL, force string) {
	time.Sleep(sim.Global.Delay())

	var (
		status  store.TransactionStatus
		finTxID string
		reason  *store.MTNErrorReason
	)
	if sim.Global.ShouldFail(force) {
		status = store.StatusFailed
		reason = &store.MTNErrorReason{Code: "NOT_ENOUGH_FUNDS", Message: "Insufficient funds in disbursement account"}
	} else {
		status = store.StatusSuccessful
		finTxID = "FIN" + strings.ToUpper(uuid.NewString()[:8])
	}

	s.UpdateDisbursementStatus(refID, status, finTxID, reason)

	if callbackURL != "" {
		tx, _ := s.GetDisbursement(refID)
		sim.FireWebhook(callbackURL, mtnCallbackPayload(tx, "disbursement"))
	}
}

// GET /mtn/disbursement/v1_0/transfer/:referenceId
func getTransfer(s *store.MTNStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		refID := c.Params("referenceId")
		tx, exists := s.GetDisbursement(refID)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(mtnErr("RESOURCE_NOT_FOUND", "Transfer not found"))
		}
		return c.JSON(mtnTxResponse(tx, "payee"))
	}
}

// GET /mtn/disbursement/v1_0/account/balance
func disbursementBalance(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"availableBalance": fmt.Sprintf("%.2f", 5_000_000+rand.Float64()*5_000_000),
		"currency":         "UGX",
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mtnErr(code, message string) fiber.Map {
	return fiber.Map{"code": code, "message": message, "error": message}
}

func envHeader(c fiber.Ctx) string {
	if env := c.Get("X-Target-Environment"); env != "" {
		return env
	}
	return "sandbox"
}

func parseBasicAuth(c fiber.Ctx) (userID, apiKey string, ok bool) {
	auth := c.Get("Authorization")
	if !strings.HasPrefix(auth, "Basic ") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// mtnTxResponse builds the status-query response body.
// partyField is "payer" for collections and "payee" for disbursements.
func mtnTxResponse(tx *store.MTNTransaction, partyField string) fiber.Map {
	resp := fiber.Map{
		"amount":                 tx.Amount,
		"currency":               tx.Currency,
		"externalId":             tx.ExternalID,
		"payerMessage":           tx.PayerMessage,
		"payeeNote":              tx.PayeeNote,
		"status":                 tx.Status,
		"financialTransactionId": tx.FinancialTransactionID,
		partyField: fiber.Map{
			"partyIdType": tx.PartyIDType,
			"partyId":     tx.PartyID,
		},
	}
	if tx.Reason != nil {
		resp["reason"] = tx.Reason
	}
	return resp
}

// mtnCallbackPayload is the webhook body sent to the caller's callback URL.
func mtnCallbackPayload(tx *store.MTNTransaction, txType string) fiber.Map {
	partyField := "payer"
	if txType == "disbursement" {
		partyField = "payee"
	}
	payload := fiber.Map{
		"referenceId":            tx.ReferenceID,
		"financialTransactionId": tx.FinancialTransactionID,
		"externalId":             tx.ExternalID,
		"amount":                 tx.Amount,
		"currency":               tx.Currency,
		"payerMessage":           tx.PayerMessage,
		"payeeNote":              tx.PayeeNote,
		"status":                 tx.Status,
		partyField: fiber.Map{
			"partyIdType": tx.PartyIDType,
			"partyId":     tx.PartyID,
		},
	}
	if tx.Reason != nil {
		payload["reason"] = tx.Reason
	}
	return payload
}
