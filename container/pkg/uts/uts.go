package uts

import (
	"github.com/google/uuid"
)

func GetRandHostName() string {
	u := uuid.New()
	return u.String()[:8]
}
