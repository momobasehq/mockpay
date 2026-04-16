package store

import (
	"sync"
	"time"
)

// MTNAPIUser represents a provisioned sandbox API user.
type MTNAPIUser struct {
	UserID               string
	APIKey               string
	ProviderCallbackHost string
}

// MTNTokenRecord holds an active access token and its scope.
type MTNTokenRecord struct {
	Token     string
	ExpiresAt time.Time
	OwnerID   string
	Scope     string // "collection" or "disbursement"
}

// MTNErrorReason is the structured error detail returned on failed transactions.
type MTNErrorReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MTNTransaction is used for both collections (requesttopay) and disbursements (transfer).
type MTNTransaction struct {
	ReferenceID            string            `json:"referenceId"`
	Amount                 string            `json:"amount"`
	Currency               string            `json:"currency"`
	FinancialTransactionID string            `json:"financialTransactionId,omitempty"`
	ExternalID             string            `json:"externalId,omitempty"`
	PartyIDType            string            `json:"partyIdType"`
	PartyID                string            `json:"partyId"`
	PayerMessage           string            `json:"payerMessage,omitempty"`
	PayeeNote              string            `json:"payeeNote,omitempty"`
	Status                 TransactionStatus `json:"status"`
	Reason                 *MTNErrorReason   `json:"reason,omitempty"`
	CallbackURL            string            `json:"-"`
	Environment            string            `json:"environment"`
	CreatedAt              time.Time         `json:"createdAt"`
}

// MTNStore is the in-memory data store for all MTN-related state.
type MTNStore struct {
	mu            sync.RWMutex
	APIUsers      map[string]*MTNAPIUser
	Tokens        map[string]*MTNTokenRecord
	Collections   map[string]*MTNTransaction
	Disbursements map[string]*MTNTransaction
}

// NewMTNStore initialises the store with a default sandbox API user for convenience.
func NewMTNStore() *MTNStore {
	s := &MTNStore{
		APIUsers:      make(map[string]*MTNAPIUser),
		Tokens:        make(map[string]*MTNTokenRecord),
		Collections:   make(map[string]*MTNTransaction),
		Disbursements: make(map[string]*MTNTransaction),
	}
	// Seed a ready-to-use default API user so callers don't have to provision one.
	s.APIUsers["mock-api-user"] = &MTNAPIUser{
		UserID:               "mock-api-user",
		APIKey:               "mock-api-key",
		ProviderCallbackHost: "localhost",
	}
	return s
}

// ---- API User management ----

func (s *MTNStore) GetAPIUser(userID string) (*MTNAPIUser, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.APIUsers[userID]
	return u, ok
}

func (s *MTNStore) CreateAPIUser(u *MTNAPIUser) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.APIUsers[u.UserID] = u
}

func (s *MTNStore) SetAPIKey(userID, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.APIUsers[userID]
	if !ok {
		return false
	}
	u.APIKey = key
	return true
}

func (s *MTNStore) ValidateCredentials(userID, apiKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.APIUsers[userID]
	if !ok {
		return false
	}
	return u.APIKey == apiKey
}

// ---- Token management ----

func (s *MTNStore) StoreToken(t *MTNTokenRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tokens[t.Token] = t
}

func (s *MTNStore) ValidateToken(token, scope string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.Tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(t.ExpiresAt) {
		return false
	}
	if scope != "" && t.Scope != scope {
		return false
	}
	return true
}

// ---- Collection (requesttopay) ----

func (s *MTNStore) SaveCollection(tx *MTNTransaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Collections[tx.ReferenceID] = tx
}

func (s *MTNStore) GetCollection(refID string) (*MTNTransaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tx, ok := s.Collections[refID]
	return tx, ok
}

func (s *MTNStore) UpdateCollectionStatus(refID string, status TransactionStatus, finTxID string, reason *MTNErrorReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx, ok := s.Collections[refID]; ok {
		tx.Status = status
		tx.FinancialTransactionID = finTxID
		tx.Reason = reason
	}
}

// ---- Disbursement (transfer) ----

func (s *MTNStore) SaveDisbursement(tx *MTNTransaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Disbursements[tx.ReferenceID] = tx
}

func (s *MTNStore) GetDisbursement(refID string) (*MTNTransaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tx, ok := s.Disbursements[refID]
	return tx, ok
}

func (s *MTNStore) UpdateDisbursementStatus(refID string, status TransactionStatus, finTxID string, reason *MTNErrorReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx, ok := s.Disbursements[refID]; ok {
		tx.Status = status
		tx.FinancialTransactionID = finTxID
		tx.Reason = reason
	}
}

// ---- Admin / inspection ----

func (s *MTNStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tokens = make(map[string]*MTNTokenRecord)
	s.Collections = make(map[string]*MTNTransaction)
	s.Disbursements = make(map[string]*MTNTransaction)
}

func (s *MTNStore) Dump() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cols := make(map[string]*MTNTransaction, len(s.Collections))
	for k, v := range s.Collections {
		cols[k] = v
	}
	disbs := make(map[string]*MTNTransaction, len(s.Disbursements))
	for k, v := range s.Disbursements {
		disbs[k] = v
	}
	return map[string]interface{}{
		"collections":   cols,
		"disbursements": disbs,
	}
}
