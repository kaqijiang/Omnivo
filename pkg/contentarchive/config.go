package contentarchive

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// ConfigFromEnv reads deployment secrets separately from administrator options.
// Without brokers the feature stays unavailable and requires no infrastructure.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Topic: "omnivo.request-content.v1", Group: "omnivo-content-archiver",
		S3Region: "us-east-1", MaxBufferedBytes: 32 << 20, MaxActive: 1024,
		ChunkBytes: 32 << 10, FlushInterval: 100 * time.Millisecond,
		MaxBatchBytes: 4 << 20, MaxPollRecords: 256,
	}
	for broker := range strings.SplitSeq(os.Getenv("CONTENT_ARCHIVE_BROKERS"), ",") {
		if broker = strings.TrimSpace(broker); broker != "" {
			cfg.Brokers = append(cfg.Brokers, broker)
		}
	}
	for name, dest := range map[string]*string{
		"TOPIC": &cfg.Topic, "GROUP": &cfg.Group, "INDEX_DSN": &cfg.IndexDSN,
		"S3_ENDPOINT": &cfg.S3Endpoint, "S3_REGION": &cfg.S3Region, "S3_BUCKET": &cfg.S3Bucket,
		"S3_ACCESS_KEY": &cfg.S3AccessKey, "S3_SECRET_KEY": &cfg.S3SecretKey,
		"KAFKA_CA_FILE": &cfg.KafkaCAFile, "SASL_MECHANISM": &cfg.SASLMechanism,
		"SASL_USERNAME": &cfg.SASLUsername, "SASL_PASSWORD": &cfg.SASLPassword,
	} {
		if value, exists := os.LookupEnv("CONTENT_ARCHIVE_" + name); exists {
			*dest = value
		}
	}
	for name, dest := range map[string]*bool{"S3_PATH_STYLE": &cfg.S3PathStyle, "KAFKA_TLS": &cfg.KafkaTLS} {
		if value, exists := os.LookupEnv("CONTENT_ARCHIVE_" + name); exists {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return Config{}, fmt.Errorf("invalid CONTENT_ARCHIVE_%s", name)
			}
			*dest = parsed
		}
	}
	for name, dest := range map[string]*int{
		"MAX_ACTIVE": &cfg.MaxActive, "CHUNK_BYTES": &cfg.ChunkBytes,
		"MAX_BATCH_BYTES": &cfg.MaxBatchBytes, "MAX_POLL_RECORDS": &cfg.MaxPollRecords,
	} {
		if value, exists := os.LookupEnv("CONTENT_ARCHIVE_" + name); exists {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return Config{}, fmt.Errorf("invalid CONTENT_ARCHIVE_%s", name)
			}
			*dest = parsed
		}
	}
	if value, exists := os.LookupEnv("CONTENT_ARCHIVE_MAX_BUFFERED_BYTES"); exists {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return Config{}, errors.New("invalid CONTENT_ARCHIVE_MAX_BUFFERED_BYTES")
		}
		cfg.MaxBufferedBytes = parsed
	}
	if value, exists := os.LookupEnv("CONTENT_ARCHIVE_FLUSH_INTERVAL_MS"); exists {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 10 || parsed > 5000 {
			return Config{}, errors.New("CONTENT_ARCHIVE_FLUSH_INTERVAL_MS must be between 10 and 5000")
		}
		cfg.FlushInterval = time.Duration(parsed) * time.Millisecond
	}
	if key := os.Getenv("CONTENT_ARCHIVE_KEY"); key != "" {
		decoded, err := base64.StdEncoding.Strict().DecodeString(key)
		if err != nil || len(decoded) != 32 {
			return Config{}, errors.New("CONTENT_ARCHIVE_KEY must encode exactly 32 bytes using standard Base64")
		}
		cfg.EncryptionKey = decoded
	}
	cfg.SASLMechanism = strings.ToUpper(strings.TrimSpace(cfg.SASLMechanism))
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) Enabled() bool { return len(cfg.Brokers) != 0 }

// Validate checks gateway and queue settings. Index and object-store constructors
// additionally validate their own settings, allowing a publisher-only process.
func (cfg Config) Validate() error {
	if !cfg.Enabled() {
		return nil
	}
	if len(cfg.EncryptionKey) != 32 {
		return errors.New("content archive encryption key must contain 32 bytes")
	}
	if len(cfg.Brokers) > 32 {
		return errors.New("too many content archive brokers")
	}
	for _, broker := range cfg.Brokers {
		host, port, err := net.SplitHostPort(broker)
		n, parseErr := strconv.Atoi(port)
		if err != nil || parseErr != nil || host == "" || strings.ContainsAny(host, "/@ \t\r\n") || n < 1 || n > 65535 {
			return errors.New("content archive brokers must be host:port addresses")
		}
	}
	if cfg.Topic == "" || cfg.Topic == "." || cfg.Topic == ".." || len(cfg.Topic) > 249 {
		return errors.New("invalid content archive topic")
	}
	for _, ch := range cfg.Topic {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '.' && ch != '-' && ch != '_' {
			return errors.New("invalid content archive topic")
		}
	}
	if cfg.Group == "" || len(cfg.Group) > 249 || strings.ContainsAny(cfg.Group, "\r\n\x00") {
		return errors.New("invalid content archive consumer group")
	}
	if cfg.ChunkBytes < 1024 || cfg.ChunkBytes > 256<<10 {
		return errors.New("content archive chunk bytes must be between 1024 and 262144")
	}
	if cfg.MaxBufferedBytes < 1<<20 || cfg.MaxBufferedBytes > 1<<30 || cfg.MaxBufferedBytes < int64(cfg.ChunkBytes)*8 {
		return errors.New("content archive buffer must be between 1 MiB and 1 GiB and hold at least eight chunks")
	}
	if cfg.MaxActive < 1 || cfg.MaxActive > 4096 {
		return errors.New("content archive active captures must be between 1 and 4096")
	}
	if cfg.FlushInterval < 10*time.Millisecond || cfg.FlushInterval > 5*time.Second {
		return errors.New("content archive flush interval must be between 10 ms and 5 s")
	}
	if cfg.MaxBatchBytes < 256<<10 || cfg.MaxBatchBytes > 16<<20 || cfg.MaxBatchBytes < cfg.ChunkBytes*2 {
		return errors.New("content archive batch must be between 256 KiB and 16 MiB and hold at least two chunks")
	}
	if cfg.MaxPollRecords < 1 || cfg.MaxPollRecords > 4096 {
		return errors.New("content archive poll records must be between 1 and 4096")
	}
	if cfg.KafkaCAFile != "" && !cfg.KafkaTLS {
		return errors.New("content archive custom CA requires Kafka TLS")
	}
	switch cfg.SASLMechanism {
	case "":
		if cfg.SASLUsername != "" || cfg.SASLPassword != "" {
			return errors.New("content archive SASL credentials require a mechanism")
		}
	case "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512":
		if cfg.SASLUsername == "" || cfg.SASLPassword == "" {
			return errors.New("content archive SASL requires username and password")
		}
	default:
		return errors.New("unsupported content archive SASL mechanism")
	}
	return nil
}

// KafkaOptions contains shared network and authentication options only. Producer
// and consumer roles add their own buffering, confirmation and group settings.
func KafkaOptions(cfg Config) ([]kgo.Opt, error) {
	if !cfg.Enabled() {
		return nil, errors.New("content archive brokers are not configured")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	opts := []kgo.Opt{kgo.SeedBrokers(cfg.Brokers...), kgo.ClientID("omnivo-content-archive"), kgo.DialTimeout(5 * time.Second), kgo.RequestTimeoutOverhead(5 * time.Second)}
	if cfg.KafkaTLS {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		if cfg.KafkaCAFile != "" {
			pem, err := os.ReadFile(cfg.KafkaCAFile)
			if err != nil || len(pem) > 1<<20 {
				return nil, errors.New("could not read content archive Kafka CA")
			}
			pool, err := x509.SystemCertPool()
			if err != nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(pem) {
				return nil, errors.New("invalid content archive Kafka CA")
			}
			tlsConfig.RootCAs = pool
		}
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	}
	switch cfg.SASLMechanism {
	case "PLAIN":
		opts = append(opts, kgo.SASL(plain.Auth{User: cfg.SASLUsername, Pass: cfg.SASLPassword}.AsMechanism()))
	case "SCRAM-SHA-256":
		opts = append(opts, kgo.SASL(scram.Auth{User: cfg.SASLUsername, Pass: cfg.SASLPassword}.AsSha256Mechanism()))
	case "SCRAM-SHA-512":
		opts = append(opts, kgo.SASL(scram.Auth{User: cfg.SASLUsername, Pass: cfg.SASLPassword}.AsSha512Mechanism()))
	}
	return opts, nil
}
