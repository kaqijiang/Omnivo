package contentarchive

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/tidwall/gjson"
)

const (
	// MaxPlainBytes bounds codec allocations independently of deployment input.
	MaxPlainBytes  int64 = 32 << 20
	maxCodecEvents       = 4096
	codecHeader          = "OMNVCA01"
)

// Keep only a small fixed number of compression workspaces. Each caller owns
// its encoder exclusively; output buffers and body slices are never pooled.
// Active encode calls are separately bounded by the publisher/archiver workers.
var archiveCompressors = make(chan *gzip.Writer, 4)

// ValidateEvent rejects malformed queue records before indexing or allocating
// unbounded data. Identity authorization remains the responsibility of the API.
func ValidateEvent(event Event) error {
	if event.Version != EventVersion || event.RequestID == "" || len(event.RequestID) > 64 || strings.TrimSpace(event.RequestID) != event.RequestID || strings.ContainsAny(event.RequestID, "\r\n\x00/") {
		return errors.New("invalid content archive event identity")
	}
	if event.Sequence < 0 || event.Sequence > 1<<40 || event.UserID < 0 || event.TokenID < 0 || event.HTTPStatus < 0 || event.HTTPStatus > 599 {
		return errors.New("invalid content archive event metadata")
	}
	if event.CreatedAt <= 0 || event.ExpiresAt <= event.CreatedAt || event.ExpiresAt-event.CreatedAt > 366*86400 {
		return errors.New("invalid content archive event retention")
	}
	if len(event.Data) > 256<<10 || len(event.Model) > 512 || len(event.Path) > 2048 || len(event.Reason) > 128 || len(event.Outcome) > 64 {
		return errors.New("content archive event exceeds its limit")
	}
	switch event.Kind {
	case EventStart:
		if event.Sequence != 0 || len(event.Data) != 0 || event.Direction != "" {
			return errors.New("invalid content archive start event")
		}
	case EventData:
		if event.Sequence == 0 || (event.Direction != DirectionRequest && event.Direction != DirectionResponse) || len(event.Data) == 0 {
			return errors.New("invalid content archive data event")
		}
	case EventEnd:
		if event.Sequence == 0 || len(event.Data) != 0 || event.Direction != "" {
			return errors.New("invalid content archive end event")
		}
	default:
		return errors.New("invalid content archive event kind")
	}
	return nil
}

// EncodeEvents compresses then authenticates the entire versioned event batch.
// Kafka and object storage therefore never receive plaintext content. A fresh
// nonce is used on every encoding, including retries that reuse an object key.
func EncodeEvents(events []Event, key []byte) ([]byte, error) {
	if len(key) != 32 || len(events) == 0 || len(events) > maxCodecEvents {
		return nil, errors.New("invalid content archive encoding parameters")
	}
	var upperBound int64
	for _, event := range events {
		if err := ValidateEvent(event); err != nil {
			return nil, err
		}
		// JSON escapes each metadata byte at most six-fold and base64 expands
		// body bytes four-for-three. Bound before allocating the JSON buffer.
		upperBound += int64(len(event.Data)+2)/3*4 + int64(len(event.RequestID)+len(event.Model)+len(event.Path)+len(event.Reason)+len(event.Outcome))*6 + 1024
		if upperBound > MaxPlainBytes {
			return nil, errors.New("content archive batch exceeds its limit")
		}
	}
	plain, err := common.Marshal(events)
	if err != nil || int64(len(plain)) > MaxPlainBytes {
		return nil, errors.New("could not encode content archive batch")
	}
	var compressed bytes.Buffer
	var zipper *gzip.Writer
	select {
	case zipper = <-archiveCompressors:
	default:
		zipper, err = gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		if err != nil {
			return nil, errors.New("could not initialize content archive compression")
		}
	}
	zipper.Reset(&compressed)
	defer func() {
		// Drop the output reference before caching the workspace, including on
		// errors. A large archive batch must not pin its bytes.Buffer in the pool.
		zipper.Reset(io.Discard)
		select {
		case archiveCompressors <- zipper:
		default:
		}
	}()
	if _, err := zipper.Write(plain); err != nil {
		_ = zipper.Close()
		return nil, errors.New("could not compress content archive batch")
	}
	if err := zipper.Close(); err != nil {
		return nil, errors.New("could not finish content archive compression")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("could not initialize content archive encryption")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("could not initialize content archive encryption")
	}
	result := make([]byte, len(codecHeader)+aead.NonceSize(), len(codecHeader)+aead.NonceSize()+compressed.Len()+aead.Overhead())
	copy(result, codecHeader)
	nonce := result[len(codecHeader):]
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.New("could not generate content archive nonce")
	}
	return aead.Seal(result, nonce, compressed.Bytes(), []byte(codecHeader)), nil
}

// DecodeEvents authenticates before decompressing and applies a caller limit in
// addition to the global hard limit. Invalid payloads never appear in errors.
func DecodeEvents(data, key []byte, maxPlainBytes int64) ([]Event, error) {
	if len(key) != 32 || maxPlainBytes < 1 || maxPlainBytes > MaxPlainBytes || int64(len(data)) > MaxPlainBytes+1<<20 || !bytes.HasPrefix(data, []byte(codecHeader)) {
		return nil, errors.New("invalid content archive envelope")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("could not initialize content archive decryption")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil || len(data) < len(codecHeader)+aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("invalid content archive envelope")
	}
	nonce := data[len(codecHeader) : len(codecHeader)+aead.NonceSize()]
	compressed, err := aead.Open(nil, nonce, data[len(codecHeader)+aead.NonceSize():], []byte(codecHeader))
	if err != nil {
		return nil, errors.New("content archive authentication failed")
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, errors.New("invalid content archive compression")
	}
	defer reader.Close()
	plain, err := io.ReadAll(io.LimitReader(reader, maxPlainBytes+1))
	if err != nil || int64(len(plain)) > maxPlainBytes {
		return nil, errors.New("content archive decoded size exceeds its limit or data is invalid")
	}
	// Bound the number of typed events before Unmarshal allocates their slice.
	// A small valid JSON array of millions of empty objects otherwise expands
	// far beyond the plaintext byte limit when decoded into Event structures.
	if common.GetJsonType(plain) != "array" || !gjson.ValidBytes(plain) {
		return nil, errors.New("invalid content archive event batch")
	}
	count := gjson.GetBytes(plain, "#").Int()
	if count < 1 || count > maxCodecEvents {
		return nil, errors.New("invalid content archive event count")
	}
	var events []Event
	if err := common.Unmarshal(plain, &events); err != nil || len(events) == 0 || len(events) > maxCodecEvents {
		return nil, errors.New("invalid content archive event batch")
	}
	for _, event := range events {
		if err := ValidateEvent(event); err != nil {
			return nil, err
		}
	}
	return events, nil
}
