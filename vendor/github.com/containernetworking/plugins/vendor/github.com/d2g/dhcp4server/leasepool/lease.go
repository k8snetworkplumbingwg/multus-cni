package leasepool

import (
	"encoding/json"
	"net"
	"time"
)

type LeaseStatus int

const (
	Free     LeaseStatus = 0
	Reserved LeaseStatus = 1
	Active   LeaseStatus = 2
)

type Lease struct {
	IP         net.IP           //The IP of the Lease
	Status     LeaseStatus      //Are Reserved, Active or Free
	MACAddress net.HardwareAddr //Mac Address of the Device
	Hostname   string           //Hostname From option 12
	Expiry     time.Time        //Expiry Time
}

func (this Lease) MarshalJSON() ([]byte, error) {
	stringMarshal := struct {
		IP         string
		Status     int
		MACAddress string
		Hostname   string
		Expiry     time.Time
	}{
		(this.IP.String()),
		int(this.Status),
		(this.MACAddress.String()),
		this.Hostname,
		this.Expiry,
	}

	return json.Marshal(stringMarshal)
}

func (this *Lease) UnmarshalJSON(data []byte) error {
	stringUnMarshal := struct {
		IP         string
		Status     int
		MACAddress string
		Hostname   string
		Expiry     time.Time
	}{}

	err := json.Unmarshal(data, &stringUnMarshal)
	if err != nil {
		return err
	}

	this.IP = net.ParseIP(stringUnMarshal.IP)
	this.Status = LeaseStatus(stringUnMarshal.Status)
	if stringUnMarshal.MACAddress != "" {
		this.MACAddress, err = net.ParseMAC(stringUnMarshal.MACAddress)
	}

	if err != nil {
		return err
	}

	this.Hostname = stringUnMarshal.Hostname
	this.Expiry = stringUnMarshal.Expiry

	return nil
}

func (this Lease) Equal(other Lease) bool {
	if !this.IP.Equal(other.IP) {
		return false
	}

	if int(this.Status) != int(other.Status) {
		return false
	}

	if this.MACAddress.String() != other.MACAddress.String() {
		return false
	}

	if this.Hostname != other.Hostname {
		return false
	}

	if !this.Expiry.Equal(other.Expiry) {
		return false
	}

	return true
}
