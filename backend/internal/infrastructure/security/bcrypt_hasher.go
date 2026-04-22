package security

import "golang.org/x/crypto/bcrypt"

// BcryptHasher 负责密码哈希与比对。
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher 创建密码哈希器。
func NewBcryptHasher(cost int) *BcryptHasher {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}

	return &BcryptHasher{cost: cost}
}

// Hash 对明文密码进行哈希。
func (h *BcryptHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

// Compare 对比哈希和明文是否匹配。
func (h *BcryptHasher) Compare(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
