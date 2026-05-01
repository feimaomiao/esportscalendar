package middleware

// Cache is the minimal Redis surface the handlers depend on. Implemented in
// production by *RedisCache; substituted in tests by an in-memory fake.
type Cache interface {
	GetBytes(key string) ([]byte, bool)
	SetBytes(key string, value []byte) error
	GetData(key string) (string, bool)
	SetData(key string, value string) error
	GetICS(hash string) (string, bool)
	SetICS(hash string, content string) error
	DeleteICS(hash string) error
	Close() error
}
