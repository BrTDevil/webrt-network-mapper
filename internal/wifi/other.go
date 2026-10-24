//go:build !linux && !windows && !darwin

package wifi

import (
	"context"
	"errors"
)

func scanOS(ctx context.Context) ([]Network, error) {
	return nil, errors.New("scanarea Wi-Fi nu este suportată pe acest sistem")
}
