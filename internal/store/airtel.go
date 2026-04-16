package store

import (
	"maps"
	"sync"
	"time"
)

// AirtelTokenRecord holds an active Airtel access token.
type AirtelTokenRecord struct {
	AccessToken string
	ExpiresAt   time.Time
	ClientID    string
}

// AirtelTransaction is used for both collections (payments) and disbursements.
type AirtelTransaction struct {
	ID            string            `json:"id"`
	AirtelMoneyID string            `json:"airtelMoneyId,omitempty"`
	Reference     string            `json:"reference,omitempty"`
	Amount        string            `json:"amount"`
	Currency      string            `json:"currency"`
	Country       string            `json:"country,omitempty"`
	MSISDN        string            `json:"msisdn"`
	Status        TransactionStatus `json:"status"`
	Message       string            `json:"message"`
	CallbackURL   string            `json:"-"`
	CreatedAt     time.Time         `json:"createdAt"`
}

// AirtelStore is the in-memory data store for all Airtel-related state.
type AirtelStore struct {
	mu            sync.RWMutex
	Tokens        map[string]*AirtelTokenRecord
	Payments      map[string]*AirtelTransaction
	Disbursements map[string]*AirtelTransaction
	Refunds       map[string]*AirtelTransaction
}

// NewAirtelStore initialises the store.
func NewAirtelStore() *AirtelStore {
	return &AirtelStore{
		Tokens:        make(map[string]*AirtelTokenRecord),
		Payments:      make(map[string]*AirtelTransaction),
		Disbursements: make(map[string]*AirtelTransaction),
		Refunds:       make(map[string]*AirtelTransaction),
	}
}

// ---- Token management ----

func (s *AirtelStore) StoreToken(t *AirtelTokenRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tokens[t.AccessToken] = t
}

func (s *AirtelStore) ValidateToken(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.Tokens[token]
	if !ok {
		return false
	}
	return time.Now().Before(t.ExpiresAt)
}

// ---- Payments (collections) ----

func (s *AirtelStore) SavePayment(tx *AirtelTransaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Payments[tx.ID] = tx
}

func (s *AirtelStore) GetPayment(id string) (*AirtelTransaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tx, ok := s.Payments[id]
	return tx, ok
}

func (s *AirtelStore) UpdatePaymentStatus(id string, status TransactionStatus, airtelMoneyID, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx, ok := s.Payments[id]; ok {
		tx.Status = status
		tx.AirtelMoneyID = airtelMoneyID
		tx.Message = message
	}
}

// ---- Disbursements ----

func (s *AirtelStore) SaveDisbursement(tx *AirtelTransaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Disbursements[tx.ID] = tx
}

func (s *AirtelStore) GetDisbursement(id string) (*AirtelTransaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tx, ok := s.Disbursements[id]
	return tx, ok
}

func (s *AirtelStore) UpdateDisbursementStatus(id string, status TransactionStatus, airtelMoneyID, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx, ok := s.Disbursements[id]; ok {
		tx.Status = status
		tx.AirtelMoneyID = airtelMoneyID
		tx.Message = message
	}
}

// ---- Refunds ----

func (s *AirtelStore) SaveRefund(tx *AirtelTransaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Refunds[tx.ID] = tx
}

func (s *AirtelStore) GetRefund(id string) (*AirtelTransaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tx, ok := s.Refunds[id]
	return tx, ok
}

// ---- Admin / inspection ----

func (s *AirtelStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tokens = make(map[string]*AirtelTokenRecord)
	s.Payments = make(map[string]*AirtelTransaction)
	s.Disbursements = make(map[string]*AirtelTransaction)
	s.Refunds = make(map[string]*AirtelTransaction)
}

func (s *AirtelStore) Dump() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pmts := make(map[string]*AirtelTransaction, len(s.Payments))
	maps.Copy(pmts, s.Payments)

	disbs := make(map[string]*AirtelTransaction, len(s.Disbursements))
	maps.Copy(disbs, s.Disbursements)

	refs := make(map[string]*AirtelTransaction, len(s.Refunds))
	maps.Copy(refs, s.Refunds)

	return map[string]any{
		"payments":      pmts,
		"disbursements": disbs,
		"refunds":       refs,
	}
}
