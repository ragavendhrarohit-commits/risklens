//go:build windows

package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wtsActive       = 0
	wtsConnected    = 1
	wtsUserName     = 5
	wtsDomainName   = 7
	wtsSessionInfo  = 24
	usernameLength  = 21
	domainLength    = 17
	winStationLen   = 32
	createNoWindow  = 0x08000000
	createUnicodeEnv = 0x00000400
	startfUseStdHandles = 0x00000100
	processQueryLimited = 0x1000
	helperFlag      = "--activity-foreground"
	helperTimeout   = 5 * time.Second
)

var (
	modWtsapi32                     = windows.NewLazySystemDLL("wtsapi32.dll")
	modUser32                       = windows.NewLazySystemDLL("user32.dll")
	modAdvapi32                     = windows.NewLazySystemDLL("advapi32.dll")
	modKernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procWTSEnumerateSessionsW       = modWtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSQuerySessionInformationW = modWtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSQueryUserToken           = modWtsapi32.NewProc("WTSQueryUserToken")
	procWTSFreeMemory               = modWtsapi32.NewProc("WTSFreeMemory")
	procWTSGetActiveConsoleSessionID = modKernel32.NewProc("WTSGetActiveConsoleSessionId")
	procGetForegroundWindow         = modUser32.NewProc("GetForegroundWindow")
	procGetWindowTextW              = modUser32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId    = modUser32.NewProc("GetWindowThreadProcessId")
	procQueryFullProcessImageNameW  = modKernel32.NewProc("QueryFullProcessImageNameW")
	procImpersonateLoggedOnUser     = modAdvapi32.NewProc("ImpersonateLoggedOnUser")
	procRevertToSelf                = modAdvapi32.NewProc("RevertToSelf")
	procProcessIdToSessionId        = modKernel32.NewProc("ProcessIdToSessionId")
)

type wtsSessionInfoW struct {
	SessionID      uint32
	WinStationName *uint16
	State          uint32
}

type wtsInfoW struct {
	State                   uint32
	SessionId               uint32
	IncomingBytes           uint32
	OutgoingBytes           uint32
	IncomingFrames          uint32
	OutgoingFrames          uint32
	IncomingCompressedBytes uint32
	OutgoingCompressedBytes uint32
	WinStationName          [winStationLen]uint16
	Domain                  [domainLength]uint16
	UserName                [usernameLength]uint16
	ConnectTime             int64
	DisconnectTime          int64
	LastInputTime           int64
	LogonTime               int64
	CurrentTime             int64
}

// WindowsCollector reads WTS sessions and the interactive foreground window.
type WindowsCollector struct{}

func (c *WindowsCollector) Collect(ctx context.Context) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	session, err := interactiveSession()
	if err != nil {
		return Observation{}, err
	}
	if session == nil {
		return Observation{}, nil
	}
	app, appErr := foregroundApp(session.WTSSessionID)
	obs := Observation{Session: session, App: app}
	if appErr != nil && app == nil {
		return obs, appErr
	}
	return obs, appErr
}

func interactiveSession() (*LiveSession, error) {
	var info *wtsSessionInfoW
	var count uint32
	r1, _, err := procWTSEnumerateSessionsW.Call(0, 0, 1, uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&count)))
	if r1 == 0 {
		return nil, fmt.Errorf("WTSEnumerateSessions: %w", err)
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(info)))

	consoleID := activeConsoleSessionID()
	var chosen *wtsSessionInfoW
	for i := uint32(0); i < count; i++ {
		row := (*wtsSessionInfoW)(unsafe.Add(unsafe.Pointer(info), uintptr(i)*unsafe.Sizeof(wtsSessionInfoW{})))
		if row.State != wtsActive && row.State != wtsConnected {
			continue
		}
		if row.SessionID == 0 {
			continue
		}
		if chosen == nil || row.SessionID == consoleID {
			chosen = row
			if row.SessionID == consoleID {
				break
			}
		}
	}
	if chosen == nil {
		return nil, nil
	}

	infoRow := querySessionInfo(chosen.SessionID)
	username, domain := queryUser(chosen.SessionID)
	if username == "" {
		username = utf16ArrayToString(infoRow.UserName[:])
	}
	if domain == "" {
		domain = utf16ArrayToString(infoRow.Domain[:])
	}
	if username == "" {
		return nil, nil
	}

	logonAt := filetimeToTime(infoRow.LogonTime)
	sid, _ := lookupSID(username, domain)
	return &LiveSession{
		WTSSessionID: chosen.SessionID,
		Username:     username,
		AccountSID:   sid,
		LogonAt:      logonAt,
	}, nil
}

func queryUser(sessionID uint32) (username, domain string) {
	username = querySessionString(sessionID, wtsUserName)
	domain = querySessionString(sessionID, wtsDomainName)
	return username, domain
}

func querySessionString(sessionID uint32, infoClass uint32) string {
	var buf *uint16
	var bytes uint32
	r1, _, _ := procWTSQuerySessionInformationW.Call(
		0,
		uintptr(sessionID),
		uintptr(infoClass),
		uintptr(unsafe.Pointer(&buf)),
		uintptr(unsafe.Pointer(&bytes)),
	)
	if r1 == 0 || buf == nil {
		return ""
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buf)))
	return windows.UTF16PtrToString(buf)
}

func querySessionInfo(sessionID uint32) wtsInfoW {
	var buf *wtsInfoW
	var bytes uint32
	r1, _, _ := procWTSQuerySessionInformationW.Call(
		0,
		uintptr(sessionID),
		uintptr(wtsSessionInfo),
		uintptr(unsafe.Pointer(&buf)),
		uintptr(unsafe.Pointer(&bytes)),
	)
	if r1 == 0 || buf == nil {
		return wtsInfoW{}
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buf)))
	return *buf
}

func activeConsoleSessionID() uint32 {
	id, _, _ := procWTSGetActiveConsoleSessionID.Call()
	return uint32(id)
}

func currentProcessSessionID() uint32 {
	var sid uint32
	r1, _, _ := procProcessIdToSessionId.Call(uintptr(windows.GetCurrentProcessId()), uintptr(unsafe.Pointer(&sid)))
	if r1 == 0 {
		return 0
	}
	return sid
}

func foregroundApp(sessionID uint32) (*LiveApp, error) {
	if currentProcessSessionID() == sessionID {
		if app := readForegroundLocal(); app != nil {
			return app, nil
		}
	}
	if app, err := readForegroundImpersonated(sessionID); err == nil && app != nil {
		return app, nil
	}
	return readForegroundViaHelper(sessionID)
}

func readForegroundImpersonated(sessionID uint32) (*LiveApp, error) {
	token, err := queryUserToken(sessionID)
	if err != nil {
		return nil, err
	}
	defer token.Close()
	r1, _, impErr := procImpersonateLoggedOnUser.Call(uintptr(token))
	if r1 == 0 {
		return nil, fmt.Errorf("ImpersonateLoggedOnUser: %w", impErr)
	}
	defer procRevertToSelf.Call()
	app := readForegroundLocal()
	if app == nil {
		return nil, fmt.Errorf("no foreground window after impersonation")
	}
	return app, nil
}

func queryUserToken(sessionID uint32) (windows.Token, error) {
	var handle windows.Handle
	r1, _, err := procWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&handle)))
	if r1 == 0 {
		return 0, fmt.Errorf("WTSQueryUserToken: %w", err)
	}
	return windows.Token(handle), nil
}

func readForegroundLocal() *LiveApp {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return nil
	}
	var titleBuf [512]uint16
	n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&titleBuf[0])), uintptr(len(titleBuf)))
	title := ""
	if n > 0 {
		title = windows.UTF16ToString(titleBuf[:n])
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	exe := ""
	if pid != 0 {
		exe = processImageName(pid)
	}
	if title == "" && exe == "" {
		return nil
	}
	return &LiveApp{
		ApplicationName: applicationName(exe, title),
		ExecutableName:  filepath.Base(exe),
		WindowTitle:     title,
	}
}

func processImageName(pid uint32) string {
	handle, err := windows.OpenProcess(processQueryLimited, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	var buf [windows.MAX_PATH]uint16
	size := uint32(len(buf))
	r1, _, _ := procQueryFullProcessImageNameW.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r1 == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:])
}

func readForegroundViaHelper(sessionID uint32) (*LiveApp, error) {
	token, err := queryUserToken(sessionID)
	if err != nil {
		return nil, err
	}
	defer token.Close()

	var primary windows.Token
	if err := windows.DuplicateTokenEx(token, windows.MAXIMUM_ALLOWED, nil, windows.SecurityImpersonation, windows.TokenPrimary, &primary); err != nil {
		return nil, fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	defer primary.Close()

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	var stdoutR, stdoutW windows.Handle
	var sa windows.SecurityAttributes
	sa.Length = uint32(unsafe.Sizeof(sa))
	sa.InheritHandle = 1
	if err := windows.CreatePipe(&stdoutR, &stdoutW, &sa, 0); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(stdoutR)
	defer windows.CloseHandle(stdoutW)
	_ = windows.SetHandleInformation(stdoutR, windows.HANDLE_FLAG_INHERIT, 0)

	desktop, err := windows.UTF16PtrFromString(`winsta0\default`)
	if err != nil {
		return nil, err
	}
	appName, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return nil, err
	}
	cmdLine, err := windows.UTF16PtrFromString(`"` + exe + `" ` + helperFlag)
	if err != nil {
		return nil, err
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop = desktop
	si.Flags = startfUseStdHandles
	si.StdOutput = stdoutW
	si.StdErr = stdoutW
	var pi windows.ProcessInformation
	if err := windows.CreateProcessAsUser(primary, appName, cmdLine, nil, nil, true, createNoWindow|createUnicodeEnv, nil, nil, &si, &pi); err != nil {
		return nil, fmt.Errorf("CreateProcessAsUser: %w", err)
	}
	defer windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)
	_ = windows.CloseHandle(stdoutW)
	stdoutW = 0

	done := make(chan struct{})
	go func() {
		_, _ = windows.WaitForSingleObject(pi.Process, uint32(helperTimeout.Milliseconds()))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(helperTimeout):
		_ = windows.TerminateProcess(pi.Process, 1)
		return nil, fmt.Errorf("foreground helper timed out")
	}

	var chunks []byte
	buf := make([]byte, 4096)
	for {
		var n uint32
		err := windows.ReadFile(stdoutR, buf, &n, nil)
		if n > 0 {
			chunks = append(chunks, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	var app LiveApp
	if err := json.Unmarshal(chunks, &app); err != nil {
		return nil, fmt.Errorf("decode foreground helper: %w", err)
	}
	if strings.TrimSpace(app.ApplicationName+app.ExecutableName+app.WindowTitle) == "" {
		return nil, nil
	}
	return &app, nil
}

// RunForegroundHelper prints the current foreground window as JSON and exits.
func RunForegroundHelper() int {
	app := readForegroundLocal()
	if app == nil {
		app = &LiveApp{}
	}
	if err := json.NewEncoder(os.Stdout).Encode(app); err != nil {
		return 1
	}
	return 0
}

func lookupSID(username, domain string) (string, error) {
	account := username
	if domain != "" && !strings.Contains(username, `\`) {
		account = domain + `\` + username
	}
	sid, _, _, err := windows.LookupSID("", account)
	if err != nil {
		sid, _, _, err = windows.LookupSID("", username)
		if err != nil {
			return "", err
		}
	}
	return sid.String(), nil
}

func filetimeToTime(ft int64) time.Time {
	if ft <= 0 {
		return time.Time{}
	}
	const ticksPerSecond = 10_000_000
	const epochDiff = 11644473600
	sec := ft/ticksPerSecond - epochDiff
	nsec := (ft % ticksPerSecond) * 100
	return time.Unix(sec, nsec).UTC()
}

func utf16ArrayToString(vals []uint16) string {
	return strings.TrimRight(windows.UTF16ToString(vals), "\x00")
}
