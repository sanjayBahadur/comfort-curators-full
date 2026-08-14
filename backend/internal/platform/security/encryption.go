package security

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
)

var (
	ErrKeyNotFound = errors.New("encryption key not found")
	ErrKeyInactive = errors.New("encryption key is inactive")
)

type KeyID string

func NewKeyID() KeyID {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("security: crypto/rand failed: " + err.Error())
	}
	return KeyID("key_" + hex.EncodeToString(b[:]))
}

type EncryptedValue struct {
	KeyID      KeyID  `json:"key_id"`
	Algorithm  string `json:"algorithm"`
	Ciphertext []byte `json:"ciphertext"`
}

type Key struct {
	ID        KeyID
	Algorithm string
	KeyBytes  SecretBytes
	Active    bool
}

type FieldEncryptor interface {
	Encrypt(ctx context.Context, keyID KeyID, plaintext []byte) (*EncryptedValue, error)
	Decrypt(ctx context.Context, ev *EncryptedValue) ([]byte, error)
}

type KeyStore interface {
	GetActiveKey(ctx context.Context) (*Key, error)
	GetKey(ctx context.Context, id KeyID) (*Key, error)
}

type RotationHook func(ctx context.Context, oldKeyID, newKeyID KeyID) error

type KeyManager interface {
	FieldEncryptor
	KeyStore
	RotateKey(ctx context.Context, newKey *Key) (KeyID, error)
	AddRotationHook(hook RotationHook)
}

type SecretString struct {
	val string
}

func NewSecretString(val string) SecretString {
	return SecretString{val: val}
}

func (s SecretString) String() string {
	return "[redacted]"
}

func (s SecretString) GoString() string {
	return "[redacted]"
}

func (s SecretString) Value() string {
	return s.val
}

func (s SecretString) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted]")
}

func (s SecretString) MarshalText() ([]byte, error) {
	return []byte("[redacted]"), nil
}

func (s SecretString) LogValue() slog.Value {
	return slog.StringValue("[redacted]")
}

type SecretBytes struct {
	val []byte
}

func NewSecretBytes(val []byte) SecretBytes {
	return SecretBytes{val: val}
}

func (s SecretBytes) String() string {
	return "[redacted]"
}

func (s SecretBytes) GoString() string {
	return "[redacted]"
}

func (s SecretBytes) Value() []byte {
	cp := make([]byte, len(s.val))
	copy(cp, s.val)
	return cp
}

func (s SecretBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted]")
}

func (s SecretBytes) LogValue() slog.Value {
	return slog.StringValue("[redacted]")
}

type NoOpKeyManager struct {
	mu    sync.RWMutex
	keys  map[KeyID]*Key
	hooks []RotationHook
}

func NewNoOpKeyManager() *NoOpKeyManager {
	return &NoOpKeyManager{
		keys: make(map[KeyID]*Key),
	}
}

func (m *NoOpKeyManager) AddKey(key *Key) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[key.ID] = key
}

func (m *NoOpKeyManager) Encrypt(ctx context.Context, keyID KeyID, plaintext []byte) (*EncryptedValue, error) {
	m.mu.RLock()
	key, ok := m.keys[keyID]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrKeyNotFound
	}
	if !key.Active {
		return nil, ErrKeyInactive
	}
	return &EncryptedValue{
		KeyID:      keyID,
		Algorithm:  key.Algorithm,
		Ciphertext: append([]byte("enc:"), plaintext...),
	}, nil
}

func (m *NoOpKeyManager) Decrypt(ctx context.Context, ev *EncryptedValue) ([]byte, error) {
	m.mu.RLock()
	_, ok := m.keys[ev.KeyID]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrKeyNotFound
	}
	if len(ev.Ciphertext) < 4 {
		return nil, errors.New("ciphertext too short")
	}
	result := make([]byte, len(ev.Ciphertext)-4)
	copy(result, ev.Ciphertext[4:])
	return result, nil
}

func (m *NoOpKeyManager) GetActiveKey(ctx context.Context) (*Key, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, k := range m.keys {
		if k.Active {
			return k, nil
		}
	}
	return nil, ErrKeyNotFound
}

func (m *NoOpKeyManager) GetKey(ctx context.Context, id KeyID) (*Key, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	k, ok := m.keys[id]
	if !ok {
		return nil, ErrKeyNotFound
	}
	return k, nil
}

func (m *NoOpKeyManager) RotateKey(ctx context.Context, newKey *Key) (KeyID, error) {
	m.mu.Lock()
	var oldKeyID KeyID
	for id, k := range m.keys {
		if k.Active {
			k.Active = false
			oldKeyID = id
			break
		}
	}
	m.keys[newKey.ID] = newKey
	hooks := make([]RotationHook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.Unlock()

	for _, hook := range hooks {
		if err := hook(ctx, oldKeyID, newKey.ID); err != nil {
			return "", err
		}
	}

	return oldKeyID, nil
}

func (m *NoOpKeyManager) AddRotationHook(hook RotationHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}
