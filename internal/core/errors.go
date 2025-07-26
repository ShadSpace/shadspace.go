package core

import "errors"

var (
	ErrFileNotFound     = errors.New("file not found in registry")
	ErrInvalidStrategy  = errors.New("invalid replication strategy")
	ErrVerificationFailed = errors.New("proof verification failed")
	ErrNetworkTimeout   = errors.New("network operation timed out")
	ErrStorageFull      = errors.New("storage capacity exceeded")
)