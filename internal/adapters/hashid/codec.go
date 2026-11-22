package hashid

import (
	"errors"
	"fmt"

	"github.com/speps/go-hashids/v2"
)

const minLength = 32

type Codec struct {
	hashID *hashids.HashID
}

func NewCodec(salt string) (*Codec, error) {
	data := hashids.NewData()
	data.Salt = salt
	data.MinLength = minLength

	hashID, err := hashids.NewWithData(data)
	if err != nil {
		return nil, fmt.Errorf("create hashid codec: %w", err)
	}

	return &Codec{hashID: hashID}, nil
}

func (c *Codec) Encode(id int64) string {
	encoded, _ := c.hashID.EncodeInt64([]int64{id})
	return encoded
}

func (c *Codec) Decode(publicID string) (int64, error) {
	decoded, err := c.hashID.DecodeInt64WithError(publicID)
	if err != nil {
		return 0, err
	}

	if len(decoded) == 0 {
		return 0, errors.New("invalid job id")
	}

	return decoded[0], nil
}
