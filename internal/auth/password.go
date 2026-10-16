package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params — параметры argon2id.
type Params struct {
	Time    uint32 // число проходов
	Memory  uint32 // КиБ
	Threads uint8
}

// DefaultParams рассчитаны на маленький VPS (1 vCPU, 1 ГБ):
//   - m = 64 МиБ — вторая рекомендация RFC 9106 для сред с ограниченной
//     памятью; при двух одновременных вычислениях (Options.MaxHashers)
//     это не больше 128 МиБ;
//   - t = 2 — проход по 64 МиБ занимает около 50–100 мс, вход за ~0,2 с
//     незаметен человеку, а перебор дорог;
//   - p = 1 — на одном ядре потоки ничего не дают, только делят время.
//
// Параметры записываются в хеш, поэтому их можно менять: старые хеши
// продолжают проверяться со своими параметрами.
var DefaultParams = Params{Time: 2, Memory: 64 * 1024, Threads: 1}

const (
	saltLen = 16
	keyLen  = 32
)

var errBadHash = errors.New("неизвестный формат хеша пароля")

// HashPassword возвращает хеш в формате
// $argon2id$v=19$m=65536,t=2,p=1$<соль base64>$<хеш base64>.
func (s *Service) HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("генерация соли: %w", err)
	}
	p := s.params
	key := s.derive(password, salt, p, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// CheckPassword сравнивает пароль с хешем за постоянное время.
func (s *Service) CheckPassword(hash, password string) (bool, error) {
	p, salt, want, err := parseHash(hash)
	if err != nil {
		return false, err
	}
	got := s.derive(password, salt, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// derive считает argon2id, не допуская больше MaxHashers вычислений сразу.
func (s *Service) derive(password string, salt []byte, p Params, n uint32) []byte {
	s.hashers <- struct{}{}
	defer func() { <-s.hashers }()
	return argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, n)
}

// burnTime тратит столько же времени, сколько проверка настоящего пароля:
// по времени ответа нельзя понять, существует ли логин.
func (s *Service) burnTime(password string) {
	s.dummyOnce.Do(func() {
		h, err := s.HashPassword("garage-dummy-password")
		if err == nil {
			s.dummyHash = h
		}
	})
	if s.dummyHash != "" {
		_, _ = s.CheckPassword(s.dummyHash, password)
	}
}

func parseHash(hash string) (Params, []byte, []byte, error) {
	// "", "argon2id", "v=19", "m=…,t=…,p=…", соль, хеш
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, errBadHash
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return Params{}, nil, nil, errBadHash
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return Params{}, nil, nil, errBadHash
	}
	if p.Time == 0 || p.Memory == 0 || p.Threads == 0 {
		return Params{}, nil, nil, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return Params{}, nil, nil, errBadHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return Params{}, nil, nil, errBadHash
	}
	return p, salt, key, nil
}
