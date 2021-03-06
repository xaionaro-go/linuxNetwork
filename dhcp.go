package networkControl

import (
	"github.com/xaionaro-go/iscDhcp/cfg"
)

type DHCP cfg.Root
type DHCPRange cfg.Range
type DHCPSubnet struct {
	cfg.Subnet
}
type DHCPValueType cfg.ValueType

func (valueType DHCPValueType) ConfigString() string {
	return cfg.ValueType(valueType).ConfigString()
}
func (valueType DHCPValueType) ToValueType() cfg.ValueType {
	return cfg.ValueType(valueType)
}

const (
	DHCPValueType_UNKNOWN     = DHCPValueType(cfg.UNKNOWN)
	DHCPValueType_BYTEARRAY   = DHCPValueType(cfg.BYTEARRAY)
	DHCPValueType_ASCIISTRING = DHCPValueType(cfg.ASCIISTRING)
)

var IsDHCPAsciiString = cfg.IsAsciiString

func NewDHCP() *DHCP {
	return (*DHCP)(cfg.NewRoot())
}

/*
type DHCPOptionValueType int

const (
	DHCPOPT_UNKNOWN = DHCPOptionValueType(0)
	DHCPOPT_ASCII   = DHCPOptionValueType(1)
	DHCPOPT_HEX     = DHCPOptionValueType(2)
)

type DHCPCommon struct {
	NSs     NSs
	Options DHCPOptions
	Domain  Domain
}

type DHCP struct {
	DHCPCommon

	RangeStart net.IP
	RangeEnd   net.IP

	// for FWSM config only:
	IfName string
}

type DHCPOption struct {
	Id        int
	ValueType DHCPOptionValueType
	Value     []byte
}

type DHCPOptions []DHCPOption
type DHCPs []DHCP
*/
