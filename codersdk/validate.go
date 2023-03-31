package codersdk

type Validator struct {
	// ValidationErrors is all the validation errors encountered during the
	// validation process.
	ValidationErrors []ValidationError `json:"validations,omitempty"`
}

func Validate() *Validator {
	return &Validator{}
}

