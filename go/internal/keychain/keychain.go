package keychain

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

// StoreToken stores an OAuth token in the OS keychain.
func StoreToken(service, provider, token string) error {
	key := fmt.Sprintf("%s:%s", service, provider)
	return keyring.Set(service, key, token)
}

// GetToken retrieves an OAuth token from the OS keychain.
func GetToken(service, provider string) (string, error) {
	key := fmt.Sprintf("%s:%s", service, provider)
	return keyring.Get(service, key)
}

// DeleteToken removes an OAuth token from the OS keychain.
func DeleteToken(service, provider string) error {
	key := fmt.Sprintf("%s:%s", service, provider)
	return keyring.Delete(service, key)
}
