package mailer

import (
	"bytes"
	"embed"
	ht "html/template"
	tt "text/template"
	"time"

	"github.com/wneessen/go-mail"
)

//go:embed "templates"
var templateFS embed.FS

// Mailer stores the mail.Client instance to connect to SMTP server and sender info
type Mailer struct {
	client *mail.Client
	sender string
}

// New initialises a new mail.Dialer instance with the given SMTP settings
func New(host string, port int, username, password, sender string) (*Mailer, error) {
	client, err := mail.NewClient(
		host,
		mail.WithSMTPAuth(mail.SMTPAuthLogin),
		mail.WithPort(port),
		mail.WithUsername(username),
		mail.WithPassword(password),
		mail.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, err
	}

	mailer := &Mailer{
		client: client,
		sender: sender,
	}

	return mailer, nil
}

// Send takes in recipient email address, template filename and dynamic
// data of type any for the templates as any parameters
func (m *Mailer) Send(recipient, templateFile string, data any) error {
	textTmpl, err := tt.New("").ParseFS(templateFS, "templates/"+templateFile)
	if err != nil {
		return err
	}

	subject := new(bytes.Buffer)
	err = textTmpl.ExecuteTemplate(subject, "subject", data)
	if err != nil {
		return err
	}

	plainBody := new(bytes.Buffer)
	err = textTmpl.ExecuteTemplate(plainBody, "plainBody", data)
	if err != nil {
		return err
	}

	htmlTmpl, err := ht.New("").ParseFS(templateFS, "templates/"+templateFile)
	if err != nil {
		return err
	}

	htmlBody := new(bytes.Buffer)
	err = htmlTmpl.ExecuteTemplate(htmlBody, "htmlBody", data)
	if err != nil {
		return err
	}

	msg := mail.NewMsg()
	err = msg.To(recipient)
	if err != nil {
		return err
	}

	err = msg.From(m.sender)
	if err != nil {
		return err
	}

	msg.Subject(subject.String())
	msg.SetBodyString(mail.TypeTextPlain, plainBody.String())
	msg.AddAlternativeString(mail.TypeTextHTML, htmlBody.String())

	// loop for retry mechanism
	for i := 0; i < 3; i++ {
		err = m.client.DialAndSend(msg)
		if err == nil {
			return nil
		}

		if i != 3 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return err
}
