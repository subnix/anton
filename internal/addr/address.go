package addr

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sigurn/crc16"

	"github.com/xssnick/tonutils-go/address"
)

type Address [34]byte

var (
	_ json.Marshaler   = (*Address)(nil)
	_ json.Unmarshaler = (*Address)(nil)
	_ sql.Scanner      = (*Address)(nil)
	_ driver.Valuer    = (*Address)(nil)
	_ fmt.Stringer     = (*Address)(nil)
)

func (x *Address) ToTU() (*address.Address, error) {
	return address.ParseAddr(x.Base64())
}

func (x *Address) FromTU(addr *address.Address) (*Address, error) {
	if len(addr.Data()) != 32 {
		return nil, fmt.Errorf("wrong addr data length %d", addr.Data())
	}
	x[0] = addr.FlagsToByte()
	x[1] = byte(addr.Workchain())
	copy(x[2:34], addr.Data())
	return x, nil
}

func MustFromTU(a *address.Address) *Address {
	addr, err := new(Address).FromTU(a)
	if err != nil {
		panic(fmt.Sprintf("%s to address", addr))
	}
	return addr
}

var crcTable = crc16.MakeTable(crc16.CRC16_XMODEM)

func (x *Address) Checksum() uint16 {
	return crc16.Checksum(x[:], crcTable)
}

func (x *Address) String() string {
	return fmt.Sprintf("%d:%x", int8(x[1]), x[2:34])
}

func (x *Address) FromString(str string) (*Address, error) {
	split := strings.Split(str, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("wrong address format")
	}
	w, err := strconv.ParseInt(split[0], 10, 8)
	if err != nil {
		return nil, errors.Wrap(err, "parse address workchain int8")
	}
	d, err := hex.DecodeString(split[1])
	if err != nil {
		return nil, errors.Wrap(err, "parse address data hex")
	}
	return x.FromTU(address.NewAddress(0, byte(w), d))
}

func (x *Address) Base64() string {
	var xCheck [36]byte
	copy(xCheck[0:34], x[:])
	binary.BigEndian.PutUint16(xCheck[34:], x.Checksum())
	return base64.RawURLEncoding.EncodeToString(xCheck[:])
}

func (x *Address) FromBase64(b64 string) (*Address, error) {
	d, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return nil, errors.Wrap(err, "decode base64")
	}
	if len(d) != 36 {
		return nil, errors.Wrap(err, "wrong decoded address length")
	}

	copy(x[0:34], d[0:34])

	if x.Checksum() != binary.BigEndian.Uint16(d[34:36]) {
		return nil, errors.Wrap(err, "wrong address checksum")
	}

	return x, nil
}

func MustFromBase64(b64 string) *Address {
	addr, err := new(Address).FromBase64(b64)
	if err != nil {
		panic(fmt.Sprintf("%s to address", addr))
	}
	return addr
}

func (x *Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Hex    string `json:"hex"`
		Base64 string `json:"base64"`
	}{
		Hex:    x.String(),
		Base64: x.Base64(),
	})
}

func (x *Address) UnmarshalJSON(raw []byte) error {
	if _, err := x.FromBase64(string(raw)); err == nil {
		return nil
	}
	if _, err := x.FromString(string(raw)); err == nil {
		return nil
	}
	return fmt.Errorf("cannot unmarshal %s to address", string(raw))
}

func (x *Address) UnmarshalText(data []byte) error {
	return x.UnmarshalJSON(data)
}

func (x *Address) Value() (driver.Value, error) {
	if x == nil {
		return nil, nil
	}
	return x[:], nil
}

func (x *Address) Scan(value interface{}) error {
	var i sql.NullString

	if value == nil {
		return nil
	}

	if err := i.Scan(value); err != nil {
		return err
	}
	if !i.Valid {
		return fmt.Errorf("error converting type %T into address", value)
	}
	if l := len(i.String); l != 34 {
		return fmt.Errorf("wrong address length %d", l)
	}

	copy(x[0:34], i.String)
	return nil
}

func Equal(x, y *Address) bool {
	if x != nil && y != nil && bytes.Equal(x[:], y[:]) {
		return true
	}
	return false
}
