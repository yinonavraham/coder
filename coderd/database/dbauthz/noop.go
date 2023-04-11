package dbauthz

import "github.com/coder/coder/coderd/database"

func NewNoop(db database.Store) Store {
	return &fake{
		Store: db,
	}
}

type fake struct {
	database.Store
}

func (f *fake) Unwrap() database.Store {
	return f.Store
}
