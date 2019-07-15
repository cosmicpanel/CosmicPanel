package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

// Types of Licenses available, defaults to DNSONLY if a valid license is not recieved from the api
const (
	FULL    = 1
	LITE    = 2
	DNSONLY = 3
	TRIAL   = 4
)

// Configuration defines the configuration for CosmicPanel
type Configuration struct {
	// Determines if CosmicPanel should be running in debug mode. This value is ignored
	// if the debug flag is passed through command line arguments
	Debug bool

	System  *SystemConfiguration
	Panel   *PanelConfiguration
	License *LicenseConfiguration
}

// SystemConfiguration defines system configuration settings
type SystemConfiguration struct {
	// Directory of CosmicPanel
	Data string

	// The user used by CosmicPanel
	Username string

	// Definitions for the user that gets created to ensure that we can quickly access
	// this information without constantly having to do a system lookup
	User struct {
		Uid int
		Gid int
	}
}

// PanelConfiguration defines the panel configuration settings
type PanelConfiguration struct {
	// The port the panel uses
	Port int
}

// LicenseConfiguration defines license configuration settings
type LicenseConfiguration struct {
	// Indicates if the license is valid, this is checked against a license server when the daemon boots
	ValidLicense bool

	// The panel license type, DNSONLY, Full, or Lite
	LicenseType int
}

// SetDefaults configures the default values for many configuration options present in the
// structs. If these values are set in the configuration file they will be overridden
func (c *Configuration) SetDefaults() {
	c.System = &SystemConfiguration{
		Username: "cosmicpanel",
		Data:     "/usr/local/cosmicpanel",
	}
	
	c.Panel = &PanelConfiguration{
		Port: 1334,
	}
}

// SetLicenseSettings sets the license status
func (c *Configuration) SetLicenseSettings(valid bool, licenseType int) {
	c.License = &LicenseConfiguration{
		ValidLicense: valid,
		LicenseType:  licenseType,
	}
}

// ReadConfiguration reads the configuration from the provided file and returns the confgiuration
// object that can then be used
func ReadConfiguration(path string) (*Configuration, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	c := &Configuration{}

	// Replace environment variables within the configuration file with their
	// values from the host system
	b = []byte(os.ExpandEnv(string(b)))

	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, err
	}

	return c, nil
}

// EnsureUser ensures that the CosmicPanel core user exists on the system. This user will be the
// owner of all data in the root data directory and is used within containers
//
// If files are not owned by this user, there will be issues with permissions on Docker
// mount points.
func (c *Configuration) EnsureUser() (*user.User, error) {
	u, err := user.Lookup(c.System.Username)

	// if an error is returned but it isn't the unknown user error just abort
	// the process entirely. If we did find a user, return it immediately.
	if err == nil {
		return u, c.SetSystemUser(u)
	} else if _, ok := err.(user.UnknownUserError); !ok {
		return nil, err
	}

	var command = fmt.Sprintf("useradd --system --no-create-home --shell /bin/false %s", c.System.Username)

	split := strings.Split(command, " ")
	if _, err := exec.Command(split[0], split[1:]...).Output(); err != nil {
		return nil, err
	}

	if u, err := user.Lookup(c.System.Username); err != nil {
		return nil, err
	} else {
		return u, c.SetSystemUser(u)
	}
}

// SetSystemUser sets the system user into the configuration then
// writes it to the disk so that it is persisted on boot
func (c *Configuration) SetSystemUser(u *user.User) error {
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	c.System.Username = u.Username
	c.System.User.Uid = uid
	c.System.User.Gid = gid

	return c.WriteToDisk()
}

// WriteToDisk writes the configuration to the disk as a blocking operation by obtating an exclusive
// lock on the file. This prevens something else from writing at the exact same time and
// leading to bad data conditions
func (c *Configuration) WriteToDisk() error {
	f, err := os.OpenFile("config.yml", os.O_WRONLY, os.ModeExclusive)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := yaml.Marshal(&c)
	if err != nil {
		return err
	}

	if _, err := f.Write(b); err != nil {
		return err
	}

	return nil
}

// LicenseVerify contains the responses from the api
type LicenseVerify struct {
	Valid       bool `json:"valid"`
	LicenseType int  `json:"licenseType"`
}

// CheckLicense checks against the licesence validation server at https://licenses.cosmicpanel.net
func (c *Configuration) CheckLicense(dnsonly bool) {

	ip := GetOutboundIP()

	if ip != "" {
		url := fmt.Sprintf("https://licenses.cosmicpanel.net/verify?ip=%s", ip)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Fatal("NewRequest: ", err)
			c.RequestNewLicense(dnsonly)
			return false
		}

		// For control over HTTP client headers,
		// and other settings
		client := &http.Client{}

		resp, err := client.Do(req)
		if err != nil {
			log.Fatal("Do : ", err)
			c.RequestNewLicense(dnsonly)
			return false
		}

		// Close the resp.Body
		defer resp.Body.Close()

		// Fill the record with data from the json
		var record LicenseVerify

		if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
			log.Println(err)
			c.RequestNewLicense(dnsonly)
			return false
		}

		c.SetLicenseSettings(record.Valid, record.LicenseType)
	}
}

// GetOutboundIP gets the public ip
func GetOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP.String()
}

// LicenseRequest is the stucture of the license request
type LicenseRequest struct {
	LicenseType int    `json:"type"`
	IP          string `json:"ip"`
}

// RequestLicense Requests a license from the license server
func (c *Configuration) requestLicense(licenseType int) {

	ip := GetOutboundIP()

	if ip != "" {
		url := "https://licenses.cosmicpanel.net/request"
		fmt.Println("Requesting License...")

		jsonBytes, err := json.Marshal(LicenseRequest{
			LicenseType: licenseType,
			IP:          ip,
		})

		if err != nil {
			log.Fatal("RequestLicense: ", err)
			return
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal("RequestLicense: ", err)
			return
		}
		defer resp.Body.Close()
				
		
	}
}

// RequestDNSONLYLicense requests a dns only license
func (c *Configuration) RequestDNSONLYLicense() {
	c.requestLicense(3)
}

// RequestTrialLicense requests a 15 day trial license
func (c *Configuration) RequestTrialLicense() {
	c.requestLicense(4)
}

// RequestNewLicense requests a new License for dnsonly or for trial
func (c *Configuration) RequestNewLicense(dnsonly bool) {
	if dnsonly {
		c.RequestDNSONLYLicense()
	} else {
		c.RequestTrialLicense()
	}
}

