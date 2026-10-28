//go:build linux

package shellcomplete

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

// Pick renders on the controlling terminal; stdout remains only the chosen text.
// The alternate screen and tty attributes are restored on cancel, errors and
// handled termination signals. Clipboard contents are always display data.
func Pick(matches []string, validPrefix bool) (string, bool, error) {
	if os.Getenv("TERM") == "dumb" {
		return "", false, errors.New("completion needs an interactive terminal")
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", false, errors.New("no controlling terminal")
	}
	defer tty.Close()
	fd := int(tty.Fd())
	original, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return "", false, err
	}
	raw := *original
	raw.Iflag &^= unix.BRKINT | unix.ICRNL | unix.INPCK | unix.ISTRIP | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.IEXTEN | unix.ISIG
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 0
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	if err = unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
		return "", false, err
	}
	defer unix.IoctlSetTermios(fd, unix.TCSETS, original)
	fmt.Fprint(tty, "\x1b[?1049h\x1b[?25l")
	defer fmt.Fprint(tty, "\x1b[0m\x1b[?25h\x1b[?1049l")
	selected := 0
	lastWidth, lastHeight := 0, 0
	dirty := true
	for {
		if ctx.Err() != nil {
			return "", false, nil
		}
		width, height := 80, 24
		if ws, e := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ); e == nil {
			if ws.Col > 0 {
				width = int(ws.Col)
			}
			if ws.Row > 0 {
				height = int(ws.Row)
			}
		}
		if width < 30 || height < 8 {
			return "", false, errors.New("resize the terminal to at least 30 columns and 8 rows")
		}
		if width != lastWidth || height != lastHeight {
			dirty = true
			lastWidth, lastHeight = width, height
		}
		if dirty {
			renderPicker(tty, matches, selected, validPrefix, width, height)
			dirty = false
		}
		key, ok, e := readByte(fd, 80)
		if e != nil {
			return "", false, e
		}
		if !ok {
			continue
		}
		if key == 27 {
			next, more, e := readByte(fd, 45)
			if e != nil {
				return "", false, e
			}
			if !more {
				return "", false, nil
			}
			if next != '[' && next != 'O' {
				return "", false, nil
			}
			next, more, e = readByte(fd, 45)
			if e != nil {
				return "", false, e
			}
			if !more {
				return "", false, nil
			}
			switch next {
			case 'A':
				key = 16
			case 'B':
				key = 14
			default:
				continue
			}
		}
		switch key {
		case 3, 7, 4:
			return "", false, nil
		case '\r', '\n':
			if len(matches) == 0 {
				return "", false, nil
			}
			return matches[selected], true, nil
		case '\t', 14:
			if len(matches) > 0 {
				selected = (selected + 1) % len(matches)
				dirty = true
			}
		case 16:
			if len(matches) > 0 {
				selected = (selected + len(matches) - 1) % len(matches)
				dirty = true
			}
		}
	}
}

func readByte(fd, timeout int) (byte, bool, error) {
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(poll, timeout)
	if errors.Is(err, unix.EINTR) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if n == 0 {
		return 0, false, nil
	}
	if poll[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
		return 0, false, errors.New("terminal closed")
	}
	var b [1]byte
	n, err = unix.Read(fd, b[:])
	if err != nil {
		return 0, false, err
	}
	return b[0], n == 1, nil
}

func preview(text string, width int) string {
	// ASCII escaping makes width predictable and keeps ambiguous Unicode or
	// control sequences out of terminal rendering. Inserted text remains exact.
	quoted := strconv.QuoteToASCII(text)
	text = quoted[1 : len(quoted)-1]
	if width < 4 {
		return ""
	}
	if len(text) > width {
		text = text[:width-3] + "..."
	}
	return text
}

func renderPicker(tty *os.File, matches []string, selected int, valid bool, width, height int) {
	fmt.Fprint(tty, "\x1b[H\x1b[2J")
	line := func(row int, color, text string) {
		if row > height {
			return
		}
		fmt.Fprintf(tty, "\x1b[%d;1H%s%s\x1b[0m", row, color, preview(text, width-1))
	}
	line(1, "\x1b[1;35m", "ClipRAG | clipboard commands")
	line(2, "\x1b[90m", "Choose a saved command to insert into your shell.")
	if len(matches) == 0 {
		message := "No matching commands. Copy a command with ClipRAG, then try again."
		if !valid {
			message = "Type at least 3 characters at the end of a single-line command."
		}
		line(4, "\x1b[33m", message)
	} else {
		visible := height - 7
		if visible < 1 {
			visible = 1
		}
		if visible > len(matches) {
			visible = len(matches)
		}
		start := 0
		if selected >= visible {
			start = selected - visible + 1
		}
		for i := start; i < start+visible; i++ {
			prefix, color := "  ", "\x1b[37m"
			if i == selected {
				prefix, color = "> ", "\x1b[1;35m"
			}
			line(4+i-start, color, prefix+matches[i])
		}
	}
	row := height - 2
	if row < 6 {
		row = 6
	}
	line(row, "\x1b[90m", "Arrows / Tab: select   Enter: insert   Esc: cancel")
	line(row+1, "\x1b[90m", "Review the command before pressing Enter again to execute.")
}
