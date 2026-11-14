package authmail

import (
	"errors"
	"fmt"
	"net/textproto"
)

type deliveryFailure string

func (e deliveryFailure) Error() string { return string(e) }

const ErrNotDelivered deliveryFailure = "mail not delivered"

func notDelivered(err error) error {
	return fmt.Errorf("%w: %w", ErrNotDelivered, err)
}

func deliveryReply(err error) error {
	var reply *textproto.Error
	if errors.As(err, &reply) && reply.Code >= 400 && reply.Code < 600 {
		return notDelivered(err)
	}
	return err
}
func submitMessage(text *textproto.Conn, data []byte) error {
	id, err := text.Cmd("DATA")
	if err != nil {
		return notDelivered(err)
	}
	text.StartResponse(id)
	_, _, err = text.ReadResponse(354)
	text.EndResponse(id)
	if err != nil {
		return deliveryReply(err)
	}
	writer := text.DotWriter()
	if _, err = writer.Write(data); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	_, _, err = text.ReadResponse(250)
	return deliveryReply(err)
}
