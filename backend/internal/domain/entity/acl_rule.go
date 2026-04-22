package entity

import "time"

// ACLRule 表示 ACL 规则。
type ACLRule struct {
	ID                uint
	SourceID          uint
	Path              string
	VirtualPath       string
	SubjectType       string
	SubjectID         uint
	Effect            string
	Priority          int
	Read              bool
	Write             bool
	Delete            bool
	Share             bool
	InheritToChildren bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
