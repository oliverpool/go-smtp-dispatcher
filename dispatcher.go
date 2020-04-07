package dispatcher

import (
	"bytes"
	"io"
	"io/ioutil"
	"sync"

	"github.com/emersion/go-smtp"
)

// Worker can accept to receive some message
type Worker interface {
	Accept(string, smtp.MailOptions, []string) bool
	NewClient() Client
}

// Client is usually a *smtp.Client struct
type Client interface {
	Mail(string, *smtp.MailOptions) error
	Rcpt(string) error
	Data() (io.WriteCloser, error)
	Quit() error
}

// Backend implements the smtp.Backend interface
type Backend struct {
	Workers []Worker
}

// Login rejects all auth attempts
func (be *Backend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return nil, smtp.ErrAuthUnsupported
}

// AnonymousLogin accept any attempt
func (be *Backend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return &session{workers: be.Workers}, nil
}

type session struct {
	workers []Worker
	from    string
	opts    smtp.MailOptions
	to      []string
}

// Reset discards currently processed message.
func (s *session) Reset() {
	s.from = ""
	s.opts = smtp.MailOptions{}
	s.to = nil
}

func (s *session) Mail(from string, opts smtp.MailOptions) error {
	s.from = from
	s.opts = opts
	return nil
}

func (s *session) Rcpt(to string) error {
	s.to = append(s.to, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	var clients []Worker
	for _, c := range s.workers {
		if c.Accept(s.from, s.opts, s.to) {
			clients = append(clients, c)
		}
	}
	if len(clients) == 0 {
		return nil
	}
	if len(clients) == 1 {
		return s.forwardTo(clients[0].NewClient(), r)
	}

	// forward to many: buffering is needed
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(c Client) {
			defer wg.Done()
			s.forwardTo(c, bytes.NewReader(buf))
		}(c.NewClient())
	}
	wg.Wait()
	return nil
}

func (s *session) Logout() error {
	return nil
}

func (s *session) forwardTo(c Client, r io.Reader) error {
	if c == nil {
		return nil
	}
	defer c.Quit()

	err := c.Mail(s.from, &s.opts)
	if err != nil {
		return err
	}

	for _, to := range s.to {
		err = c.Rcpt(to)
		if err != nil {
			return err
		}
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}

	_, err = io.Copy(wc, r)
	if err != nil {
		wc.Close()
		return err
	}
	return wc.Close()
}
