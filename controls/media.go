package controls

import (
	webostv "github.com/extbe/webos-tv"
	"github.com/google/uuid"
)

const (
	uriVolumeUp   = "ssap://audio/volumeUp"
	uriVolumeDown = "ssap://audio/volumeDown"
	uriGetVolume  = "ssap://audio/getVolume"
	uriSetVolume  = "ssap://audio/setVolume"
)

type Media struct {
	c webostv.Client
}

func NewMedia(c webostv.Client) Media {
	return Media{c: c}
}

func (m Media) VolumeUp() error {
	msg := newRequestMessage()
	msg.URI = uriVolumeUp

	_, err := m.c.SendBlocking(msg)

	return err
}

func (m Media) VolumeDown() error {
	msg := newRequestMessage()
	msg.URI = uriVolumeDown

	_, err := m.c.SendBlocking(msg)

	return err
}

// todo: test me
//func (m Media) GetVolume() (int, error) {
//	msg := newRequestMessage()
//	msg.URI = uriGetVolume
//
//	rsp, err := m.c.SendBlocking(msg)
//	if err != nil {
//		return 0, err
//	}
//
//	return rsp.Payload
//}

func (m Media) SetVolume(level int) error {
	msg := newRequestMessage()
	msg.URI = uriSetVolume
	msg.Payload = map[string]interface{}{
		"volume": level,
	}

	_, err := m.c.SendBlocking(msg)

	return err
}

func newRequestMessage() webostv.Message {
	return webostv.Message{
		Type: webostv.RequestMsgType,
		ID:   uuid.New().String(),
	}
}
