//go:build windows

package users

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	netapiMaxPreferredLength = 0xFFFFFFFF
	filterNormalAccount      = 0x0002

	ufAccountDisable = 0x0002
	ufPasswdNotReqd  = 0x0020
	ufNormalAccount  = 0x0200

	ridAdministrator = 500
	ridGuest         = 501
)

var (
	modNetapi32          = windows.NewLazySystemDLL("netapi32.dll")
	procNetUserEnum      = modNetapi32.NewProc("NetUserEnum")
	procNetApiBufferFree = modNetapi32.NewProc("NetApiBufferFree")
)

// USER_INFO_2 layout (partial) for NetUserEnum level 2.
type userInfo2 struct {
	Name         *uint16
	Password     *uint16
	PasswordAge  uint32
	Priv         uint32
	HomeDir      *uint16
	Comment      *uint16
	Flags        uint32
	ScriptPath   *uint16
	AuthFlags    uint32
	FullName     *uint16
	UsrComment   *uint16
	Parms        *uint16
	Workstations *uint16
	LastLogon    uint32
	LastLogoff   uint32
	AcctExpires  uint32
	MaxStorage   uint32
	UnitsPerWeek uint32
	LogonHours   *byte
	BadPwCount   uint32
	NumLogons    uint32
	LogonServer  *uint16
	CountryCode  uint32
	CodePage     uint32
}

// WindowsCollector enumerates local accounts via NetUserEnum.
type WindowsCollector struct{}

// Collect returns local Windows user accounts. Partial results are returned
// when some accounts fail SID lookup.
func (c *WindowsCollector) Collect(ctx context.Context) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var (
		buf          *byte
		entriesRead  uint32
		totalEntries uint32
		resumeHandle uint32
	)

	status, _, _ := procNetUserEnum.Call(
		0, // local computer
		2, // USER_INFO_2
		uintptr(filterNormalAccount),
		uintptr(unsafe.Pointer(&buf)),
		uintptr(netapiMaxPreferredLength),
		uintptr(unsafe.Pointer(&entriesRead)),
		uintptr(unsafe.Pointer(&totalEntries)),
		uintptr(unsafe.Pointer(&resumeHandle)),
	)
	if buf != nil {
		defer procNetApiBufferFree.Call(uintptr(unsafe.Pointer(buf)))
	}
	if status != 0 && windows.Errno(status) != windows.ERROR_MORE_DATA {
		return nil, fmt.Errorf("NetUserEnum failed: %w", windows.Errno(status))
	}
	if entriesRead == 0 || buf == nil {
		return []Account{}, nil
	}

	accounts := make([]Account, 0, entriesRead)
	var partialErrs []string
	infoSize := unsafe.Sizeof(userInfo2{})

	for i := uint32(0); i < entriesRead; i++ {
		if err := ctx.Err(); err != nil {
			return accounts, err
		}
		raw := (*userInfo2)(unsafe.Add(unsafe.Pointer(buf), uintptr(i)*infoSize))
		username := utf16PtrToString(raw.Name)
		if username == "" {
			continue
		}

		sid, err := lookupSID(username)
		if err != nil {
			partialErrs = append(partialErrs, fmt.Sprintf("%s: %v", username, err))
			continue
		}

		enabled := raw.Flags&ufAccountDisable == 0
		passwordRequired := raw.Flags&ufPasswdNotReqd == 0
		builtin := isBuiltinSID(sid)
		accountType := classifyAccountType(sid, raw.Flags)

		var lastLogon *time.Time
		if raw.LastLogon > 0 {
			t := time.Unix(int64(raw.LastLogon), 0).UTC()
			lastLogon = &t
		}

		accounts = append(accounts, Account{
			Username:         username,
			SID:              sid,
			DisplayName:      utf16PtrToString(raw.FullName),
			AccountType:      accountType,
			Enabled:          enabled,
			Builtin:          builtin,
			PasswordRequired: passwordRequired,
			LastLogon:        lastLogon,
			Source:           SourceLocal,
		})
	}

	if len(partialErrs) > 0 && len(accounts) == 0 {
		return nil, fmt.Errorf("user enumeration failed: %s", strings.Join(partialErrs, "; "))
	}
	if len(partialErrs) > 0 {
		return accounts, fmt.Errorf("partial user enumeration: %s", strings.Join(partialErrs, "; "))
	}
	return accounts, nil
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	return windows.UTF16PtrToString(p)
}

func lookupSID(username string) (string, error) {
	sid, _, _, err := windows.LookupSID("", username)
	if err != nil {
		return "", err
	}
	return sid.String(), nil
}

func isBuiltinSID(sid string) bool {
	rid := sidRID(sid)
	return rid == ridAdministrator || rid == ridGuest
}

func classifyAccountType(sid string, flags uint32) string {
	rid := sidRID(sid)
	switch rid {
	case ridAdministrator:
		return AccountTypeAdmin
	case ridGuest:
		return AccountTypeGuest
	}
	if flags&ufNormalAccount != 0 {
		return AccountTypeLocal
	}
	return AccountTypeUnknown
}

func sidRID(sid string) int {
	parts := strings.Split(sid, "-")
	if len(parts) == 0 {
		return -1
	}
	var rid int
	fmt.Sscanf(parts[len(parts)-1], "%d", &rid)
	return rid
}
