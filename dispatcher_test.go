package dispatcher

import (
	"github.com/emersion/go-smtp"
)

var _ smtp.Backend = &Backend{}
var _ Client = &smtp.Client{}
