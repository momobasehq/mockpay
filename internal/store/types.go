package store

// TransactionStatus represents the lifecycle state of any transaction.
type TransactionStatus string

const (
	StatusPending    TransactionStatus = "PENDING"
	StatusSuccessful TransactionStatus = "SUCCESSFUL"
	StatusFailed     TransactionStatus = "FAILED"
	StatusCancelled  TransactionStatus = "CANCELLED"
)
