package peer

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/bits"

	"github.com/saveio/carrier/crypto/blake2b"
	"github.com/saveio/carrier/internal/protobuf"
)

// ID is an identity of nodes, using its public key hash and network address.
type ID protobuf.ID

// CreateID is a factory function creating ID.
func CreateID(address string, publicKey []byte) ID {
	return ID{
		Address: address,
		NetKey:  publicKey, Id: blake2b.New().HashBytes(publicKey),
		ConnectionId: blake2b.New().HashBytes(append(publicKey, []byte(address)...)),
	}
}

// String returns the identity address and public key.
func (id ID) String() string {
	return fmt.Sprintf("ID{Address: %v, Id: %v}", id.Address, id.Id)
}

// Equals determines if two peer IDs are equal to each other based on the contents of their public keys.
func (id ID) Equals(other ID) bool {
	return bytes.Equal(id.Id, other.Id)
}

// Less determines if this peer ID's public key is less than other ID's public key.
func (id ID) Less(other interface{}) bool {
	if other, is := other.(ID); is {
		return bytes.Compare(id.Id, other.Id) == -1
	}
	return false
}

// PublicKeyHex generates a hex-encoded string of public key hash of this given peer ID.
func (id ID) PublicKeyHex() string {
	return hex.EncodeToString(id.NetKey)
}

// Xor performs XOR (^) over another peer ID's public key.
func (id ID) Xor(other ID) ID {
	result := make([]byte, len(id.NetKey))

	for i := 0; i < len(id.NetKey) && i < len(other.NetKey); i++ {
		result[i] = id.NetKey[i] ^ other.NetKey[i]
	}
	return ID{Address: id.Address, NetKey: result}
}

// XorID performs XOR (^) over another peer ID's public key hash.
func (id ID) XorID(other ID) ID {
	result := make([]byte, len(id.Id))

	for i := 0; i < len(id.Id) && i < len(other.Id); i++ {
		result[i] = id.Id[i] ^ other.Id[i]
	}
	return ID{Address: id.Address, Id: result}
}

// PrefixLen returns the number of prefixed zeros in a peer ID.
func (id ID) PrefixLen() int {
	for i, b := range id.Id {
		if b != 0 {
			return i*8 + bits.LeadingZeros8(uint8(b))
		}
	}
	return len(id.Id)*8 - 1
}
