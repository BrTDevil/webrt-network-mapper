//go:build !linux

package bluetooth

import (
	"context"
	"errors"
	"time"
)

// Pe Windows și macOS, sistemul nu oferă un utilitar de linie de comandă pentru
// descoperirea dispozitivelor din apropiere fără împerechere.
func scanOS(ctx context.Context, dur time.Duration) ([]Device, error) {
	return nil, errors.New("scanarea Bluetooth e disponibilă deocamdată doar pe Linux (BlueZ)")
}
