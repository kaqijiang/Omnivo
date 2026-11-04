package contentarchive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var ErrContentUnavailable = errors.New("content archive data is unavailable")

type contentIndex interface {
	ListContent(context.Context, string, int64, int, int64) (ContentPage, []EventReference, error)
}

type Reader struct {
	cfg     Config
	index   contentIndex
	objects ObjectStore
}

func NewReader(cfg Config, index *Index, objects ObjectStore) *Reader {
	return &Reader{cfg: cfg, index: index, objects: objects}
}

// ReadPage reads at most 32 events and 1 MiB of body bytes. Only one bounded
// archive object is cached at a time, even when a page spans many batches.
// Authorization belongs to the caller and must precede this function.
func (r *Reader) ReadPage(ctx context.Context, requestID string, afterSequence int64, limit int) (*ContentPage, error) {
	if r.index == nil || r.objects == nil || len(r.cfg.EncryptionKey) != 32 || afterSequence < -1 || len(requestID) == 0 || len(requestID) > 64 {
		return nil, ErrContentUnavailable
	}
	if limit <= 0 {
		limit = 32
	}
	limit = min(limit, 32)
	now := time.Now().Unix()
	page, refs, err := r.index.ListContent(ctx, requestID, afterSequence, limit, now)
	if err != nil {
		return nil, ErrContentUnavailable
	}
	if page.RequestID == "" {
		return nil, nil
	}
	if page.RequestID != requestID || page.ExpiresAt <= now {
		return nil, ErrContentUnavailable
	}
	page.Events = make([]Event, 0, len(refs))
	var cachedKey, cachedChecksum string
	var cachedEvents []Event
	bytesRead := 0
	previousSequence := afterSequence
	for _, ref := range refs {
		if ref.Sequence <= previousSequence || !validObjectKey(ref.ObjectKey) || len(ref.Checksum) != 64 {
			return nil, ErrContentUnavailable
		}
		if ref.ObjectKey != cachedKey {
			// Ciphertext includes framing and compression overhead. Both the
			// transport read and decoded JSON have independent hard bounds.
			// The current producer batch size may be smaller than historical
			// objects. Retain the codec's fixed safety limits for old reads.
			data, err := r.objects.Get(ctx, ref.ObjectKey, MaxPlainBytes+(1<<20))
			if err != nil {
				return nil, ErrContentUnavailable
			}
			cachedEvents, err = DecodeEvents(data, r.cfg.EncryptionKey, MaxPlainBytes)
			if err != nil {
				return nil, ErrContentUnavailable
			}
			canonical, err := common.Marshal(cachedEvents)
			if err != nil {
				return nil, ErrContentUnavailable
			}
			digest := sha256.Sum256(canonical)
			cachedChecksum = hex.EncodeToString(digest[:])
			cachedKey = ref.ObjectKey
		}
		if cachedChecksum != ref.Checksum || ref.EventIndex < 0 || ref.EventIndex >= len(cachedEvents) {
			return nil, ErrContentUnavailable
		}
		event := cachedEvents[ref.EventIndex]
		if event.RequestID != requestID || event.Sequence != ref.Sequence || event.UserID != page.UserID || event.CreatedAt != page.CreatedAt || event.ExpiresAt != page.ExpiresAt || event.ExpiresAt <= time.Now().Unix() {
			return nil, ErrContentUnavailable
		}
		if bytesRead+len(event.Data) > 1<<20 {
			if len(page.Events) == 0 {
				return nil, ErrContentUnavailable
			}
			page.NextSequence = new(previousSequence)
			break
		}
		// Copy the selected body: keeping a tiny slice must not retain an
		// entire decoded object after switching to the next object.
		event.Data = append([]byte(nil), event.Data...)
		event.RequestID = strings.Clone(event.RequestID)
		event.Kind = strings.Clone(event.Kind)
		event.Direction = strings.Clone(event.Direction)
		event.Model = strings.Clone(event.Model)
		event.Path = strings.Clone(event.Path)
		event.Outcome = strings.Clone(event.Outcome)
		event.Reason = strings.Clone(event.Reason)
		page.Events = append(page.Events, event)
		bytesRead += len(event.Data)
		previousSequence = event.Sequence
	}
	if page.ExpiresAt <= time.Now().Unix() {
		return nil, ErrContentUnavailable
	}
	return &page, nil
}
