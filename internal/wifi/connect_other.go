//go:build !linux

package wifi

import (
	"context"
	"errors"
)

func CurrentConnection(ctx context.Context) *Current { return nil }

func Connect(ctx context.Context, ssid, password, iface string) (*Session, error) {
	return nil, errors.New("conectarea automată la altă rețea Wi-Fi e disponibilă doar pe Linux; conectează-te manual și rulează scanul")
}

func (s *Session) Restore() {}
