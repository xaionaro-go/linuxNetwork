package iptables

import (
	"encoding/json"
	"errors"
	"fmt"
	ipt "github.com/coreos/go-iptables/iptables"
	"github.com/xaionaro-go/go-ipset/ipset"
	"github.com/xaionaro-go/networkControl"
	"net"
	"os"
	"strconv"
	"strings"
)

var (
	errNotImplemented = errors.New("not implemented (yet?)")
)

type iptables struct {
	iptables              *ipt.IPTables
	isSameSecurityTraffic bool
}

func NewFirewall() networkControl.FirewallI {
	newIPT, err := ipt.New()
	if err != nil {
		panic(err)
	}

	fw := &iptables{iptables: newIPT}

	fw.iptables.NewChain("filter", "ACLs")
	fw.iptables.NewChain("filter", "SECURITY_LEVELs")
	fw.iptables.NewChain("nat", "SNATs")
	fw.iptables.NewChain("nat", "DNATs")

	fw.iptables.AppendUnique("filter", "FORWARD", "-j", "ACLs")
	fw.iptables.AppendUnique("filter", "FORWARD", "-j", "SECURITY_LEVELs")
	fw.iptables.AppendUnique("nat", "PREROUTING", "-j", "DNATs")
	fw.iptables.AppendUnique("nat", "POSTROUTING", "-j", "SNATs")

	return fw
}

func (fw iptables) InquireSecurityLevel(ifName string) int {
	setNames, err := ipset.Names()
	if err != nil {
		panic(err)
	}

	// TODO: sort setNames numberically before the "for" below

	// Searching for IFACES.SECURITY_LEVEL.###
	for _, setName := range setNames {
		if !strings.HasPrefix(setName, "IFACES.SECURITY_LEVEL.") {
			continue
		}

		setNameWords := strings.Split(setName, ".")
		securityLevel, err := strconv.Atoi(setNameWords[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid security level value \"%v\" (in ipset name: %v): %v", setNameWords[2], setName, err.Error())
			continue
		}

		rows, err := ipset.List(setName)
		if err != nil {
			panic(err)
		}

		for _, row := range rows {
			words := strings.Split(row, ",")
			if words[1] == ifName {
				return securityLevel
			}
		}
	}

	return 0
}

func (fw *iptables) createSecurityLevelRules() error {
	setNames, err := ipset.Names()
	if err != nil {
		panic(err)
	}

	// Searching for IFACES.SECURITY_LEVEL.###
	securityLevels := []int{}
	for _, setName := range setNames {
		if !strings.HasPrefix(setName, "IFACES.SECURITY_LEVEL.") {
			continue
		}

		setNameWords := strings.Split(setName, ".")
		securityLevel, err := strconv.Atoi(setNameWords[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid security level value \"%v\" (in ipset name: %v): %v", setNameWords[2], setName, err.Error())
			continue
		}

		securityLevels = append(securityLevels, securityLevel)
	}

	for _, securityLevelA := range securityLevels {

		setName := "IFACES.SECURITY_LEVEL." + strconv.Itoa(securityLevelA)
		chainName := setName

		minSecurityLevelB := -1
		for _, securityLevelB := range securityLevels {
			if securityLevelA >= securityLevelB {
				continue
			}
			if securityLevelB < minSecurityLevelB {
				minSecurityLevelB = securityLevelB
			}
		}

		setNameB := "IFACES.SECURITY_LEVEL." + strconv.Itoa(minSecurityLevelB)
		chainNameB := setNameB
		fw.iptables.AppendUnique("filter", chainName, "-j", chainNameB)

		if fw.isSameSecurityTraffic {
			fw.iptables.AppendUnique("filter", chainName, "-m", "set", "--match-set", setName, "dst,dst", "-j", "ACCEPT")
		} else {
			fw.iptables.AppendUnique("filter", chainName, "-m", "set", "--match-set", setNameB, "dst,dst", "-j", "ACCEPT")
		}

		fw.iptables.AppendUnique("filter", "SECURITY_LEVELs", "-m", "set", "--match-set", setName, "src,src", "-j", chainName)
	}

	return nil
}

func (fw *iptables) addSecurityLevel(securityLevel int) error {
	setName := "IFACES.SECURITY_LEVEL." + strconv.Itoa(securityLevel)
	_, err := ipset.New(setName, "hash:net,iface", &ipset.Params{})
	if err != nil {
		return err
	}

	return fw.createSecurityLevelRules()
}

func (fw *iptables) SetSecurityLevel(ifName string, securityLevel int) error {
	setName := "IFACES.SECURITY_LEVEL." + strconv.Itoa(securityLevel)

	// Remembering the old security level set name

	oldSecurityLevel := fw.InquireSecurityLevel(ifName)
	oldSetName := "IFACES.SECURITY_LEVEL." + strconv.Itoa(oldSecurityLevel)

	// Create the security level if not exists

	fw.addSecurityLevel(securityLevel)

	// Adding to the new security level

	err := ipset.Add(setName, "0.0.0.0/0,"+ifName, 0)
	if err != nil {
		return err
	}

	// Removing from the old security level

	ipset.Del(oldSetName, "0.0.0.0/0,"+ifName)

	return nil
}

func (fw iptables) getACLsNames() (result []string) {
	setNames, err := ipset.Names()
	if err != nil {
		panic(err)
	}
	// Searching for ACL.IN.###
	for _, setName := range setNames {
		setNameWords := strings.Split(setName, ".")
		if setNameWords[0] != "ACL" {
			continue
		}
		if len(setNameWords) != 3 {
			panic(fmt.Errorf("Internal error: %v", setNameWords))
		}
		result = append(result, setNameWords[2])
	}

	return
}

func (fw iptables) inquireACL(aclName string) (result networkControl.ACL) {
	result.Name = aclName

	// Getting VLANs of the ACL

	setName := "ACL.IN." + aclName
	setRows, err := ipset.List(setName)
	if err != nil {
		panic(err)
	}

	for _, setRow := range setRows {
		words := strings.Split(setRow, ",")
		if words[0] != "0.0.0.0/0" {
			panic("Internal error. words[0] == \"" + words[0] + "\"")
		}
		ifName := words[1]
		result.VLANNames = append(result.VLANNames, ifName)
	}

	// Getting rules of the ACL

	rules, err := fw.iptables.List("filter", "ACL.IN."+aclName)
	if err != nil {
		panic(err)
	}

	for _, rule := range rules {
		result.Rules = append(result.Rules, parseACLRule(rule))
	}

	return
}
func (fw iptables) InquireACLs() (result networkControl.ACLs) {
	aclNames := fw.getACLsNames()
	for _, aclName := range aclNames {
		acl := fw.inquireACL(aclName)
		result = append(result, &acl)
	}

	return
}
func (fw iptables) InquireSNATs() (result networkControl.SNATs) {
	ruleStrings, err := fw.iptables.List("snat", "SNATs")
	if err != nil {
		panic(err)
	}
	for _, ruleString := range ruleStrings {
		words := strings.Split(ruleString, " ")
		snat := networkControl.SNAT{}
		source := networkControl.SNATSource{}
		for len(words) > 0 {
			switch words[0] {
			case "-i":
				source.IfName = words[1]
				words = words[2:]
			case "-m":
				words = words[2:]
			case "-s":
				var err error
				source.IPNet, err = networkControl.IPNetFromCIDRString(words[1])
				if err != nil {
					panic(err)
				}
				words = words[2:]
			case "--comment":
				snatComment := snatCommentT{}
				err := json.Unmarshal([]byte(words[1]), &snatComment)
				if err != nil {
					panic(err)
				}
				snat.FWSMGlobalId = snatComment.FWSMGlobalId
				words = words[2:]
			case "-j":
				if words[1] != "SNAT" || words[2] != "--to-source" {
					panic("illegal rule: "+ruleString)
				}
				snat.NATTo = net.ParseIP(words[3])
				words = words[4:]
			default:
				panic(errNotImplemented)
			}
		}
		snat.Sources = append(snat.Sources, source)
	}
	return
}
func (fw iptables) InquireDNATs() (result networkControl.DNATs) {
	panic("Not implemented, yet")
	return
}

func ruleToNetfilterRule(rule networkControl.ACLRule) (result []string) {
	protocolString := rule.Protocol.String()
	if protocolString != "ip" {
		result = append(result, "-p", protocolString)
	}

	result = append(result, "-m", "multiport", "-s", rule.FromNet.String(), "--sports", portRangesToNetfilterPorts(rule.FromPortRanges), "-d", rule.ToNet.String(), "--dports", portRangesToNetfilterPorts(rule.ToPortRanges))

	var action string
	switch rule.Action {
	case networkControl.ACL_ALLOW:
		action = "ACCEPT"
	case networkControl.ACL_DENY:
		action = "REJECT"
	default:
		panic(fmt.Errorf("Unknown action: %v", rule))
	}

	result = append(result, "-j", action)

	return result
}

func portRangesToNetfilterPorts(portRanges networkControl.PortRanges) string {
	convPortRanges := []string{}

	for _, portRange := range portRanges {
		convPortRanges = append(convPortRanges, fmt.Sprintf("%v:%v", portRange.Start, portRange.End))
	}

	return strings.Join(convPortRanges, ",")
}

func ParseNetfilterPortRanges(portRangesString string) (portRanges networkControl.PortRanges) {
	portRangeStrings := strings.Split(portRangesString, ",")

	for _, portRangeString := range portRangeStrings {
		portRangeWords := strings.Split(portRangeString, ":")
		if len(portRangeWords) == 1 {
			portRangeWords = append(portRangeWords, portRangeWords[0])
		}

		portRangeStart, err := strconv.Atoi(portRangeWords[0])
		if err != nil {
			panic(err)
		}
		portRangeEnd, err := strconv.Atoi(portRangeWords[1])
		if err != nil {
			panic(err)
		}

		portRange := networkControl.PortRange{
			Start: uint16(portRangeStart),
			End: uint16(portRangeEnd),
		}

		portRanges = append(portRanges, portRange)
	}

	return
}

func parseACLRule(ruleString string) (rule networkControl.ACLRule) {
	words := strings.Split(ruleString, " ")
	for len(words) > 0 {
		switch words[0] {
		case "-m":
			words = words[2:]
		case "-s":
			var err error
			rule.FromNet, err = networkControl.IPNetFromCIDRString(words[1])
			if err != nil {
				panic(err)
			}
			words = words[2:]
		case "-d":
			var err error
			rule.ToNet, err = networkControl.IPNetFromCIDRString(words[1])
			if err != nil {
				panic(err)
			}
			words = words[2:]
		case "-p":
			rule.Protocol = networkControl.ProtocolFromString(words[1])
			words = words[2:]
		case "--sports":
			rule.ToPortRanges = ParseNetfilterPortRanges(words[1])
			words = words[2:]
		case "--dports":
			rule.FromPortRanges = ParseNetfilterPortRanges(words[1])
			words = words[2:]
		default:
			panic(errNotImplemented)
		}
	}

	return
}

func (fw *iptables) AddACL(acl networkControl.ACL) (err error) {

	setName := "ACL.IN." + acl.Name
	chainName := setName

	// adding an ipset

	var set *ipset.IPSet
	set, err = ipset.New(setName, "hash:net,iface", &ipset.Params{})
	if err != nil {
		return
	}
	for _, vlanName := range acl.VLANNames {
		err = set.Add("0.0.0.0/0,"+vlanName, 0)
		if err != nil {
			return
		}
	}

	// adding a chain to iptables

	err = fw.iptables.NewChain("filter", chainName)
	if err != nil {
		return err
	}
	for _, rule := range acl.Rules {
		err = fw.iptables.AppendUnique("filter", chainName, ruleToNetfilterRule(rule)...)
		if err != nil {
			return
		}
	}

	// activating the chain

	return fw.iptables.AppendUnique("filter", "ACLs", "-m", "set", "--match-set", setName, "src,src", "-j", chainName)
}

type snatCommentT struct {
	FWSMGlobalId int `json",omitempty"`
}

func (c snatCommentT) Json() string {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	return string(b)
}
func (c snatCommentT) String() string {
	return c.Json()
}

func (fw *iptables) AddSNAT(snat networkControl.SNAT) error {
	for _, source := range snat.Sources {
		snatComment := snatCommentT{
			FWSMGlobalId: snat.FWSMGlobalId,
		}
		err := fw.iptables.AppendUnique("nat", "SNATs", "-o", source.IfName, "-s", source.IPNet.String(), "-j", "SNAT", "--to-source", snat.NATTo.String(), "-m", "comment", "--comment", snatComment.Json())
		if err != nil {
			return err
		}
	}
	return nil
}

func ipportToNetfilterIPPort(ipport networkControl.IPPort, shouldAppendProto bool) string {
	if ipport.Port == nil && ipport.Protocol == nil {
		return ipport.IP.String()
	}
	if ipport.Port == nil || ipport.Protocol == nil {
		panic("This case is not implemented")
	}
	result := fmt.Sprintf("%v:%v", ipport.IP, *ipport.Port)
	if shouldAppendProto {
		result += " -p " + ipport.Protocol.String()
	}
	return result
}

func (fw *iptables) AddDNAT(dnat networkControl.DNAT) error {
	for _, destination := range dnat.Destinations {
		err := fw.iptables.AppendUnique("nat", "DNATs", "-i", dnat.IfName, "-d", ipportToNetfilterIPPort(destination, true), "-j", "DNAT", "--to-destination", ipportToNetfilterIPPort(dnat.NATTo, false))
		if err != nil {
			return err
		}
	}
	return nil
}
func (fw *iptables) UpdateACL(acl networkControl.ACL) error {
	return errNotImplemented
}
func (fw *iptables) UpdateSNAT(snat networkControl.SNAT) error {
	return errNotImplemented
}
func (fw *iptables) UpdateDNAT(dnat networkControl.DNAT) error {
	return errNotImplemented
}
func (fw *iptables) RemoveACL(acl networkControl.ACL) error {

	setName := "ACL.IN." + acl.Name
	chainName := setName

	// deactivating the chain

	err := fw.iptables.Delete("filter", "ACLs", "-m", "set", "--match-set", setName, "src,src", "-j", chainName)
	if err != nil {
		return err
	}

	// removing the chain

	err = fw.iptables.ClearChain("filter", chainName)
	if err != nil {
		return err
	}
	err = fw.iptables.DeleteChain("filter", chainName)
	if err != nil {
		return err
	}

	// removing the set

	return ipset.Destroy(setName)
}
func (fw *iptables) RemoveSNAT(snat networkControl.SNAT) error {
	for _, source := range snat.Sources {
		snatComment := snatCommentT{
			FWSMGlobalId: snat.FWSMGlobalId,
		}
		err := fw.iptables.Delete("nat", "SNATs", "-o", source.IfName, "-s", source.IPNet.String(), "-j SNAT --to-source", snat.NATTo.String(), "-m", "comment", "--comment", snatComment.Json())
		if err != nil {
			return err
		}
	}
	return nil
}
func (fw *iptables) RemoveDNAT(dnat networkControl.DNAT) error {
	for _, destination := range dnat.Destinations {
		err := fw.iptables.Delete("nat", "DNATs", "-i", dnat.IfName, "-d", ipportToNetfilterIPPort(destination, true), "-j", "DNAT", "--to-destination", ipportToNetfilterIPPort(dnat.NATTo, false))
		if err != nil {
			return err
		}
	}
	return nil
}
