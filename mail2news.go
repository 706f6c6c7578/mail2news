package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/net/proxy"
)

const (
	server         = "peannyjkqwqfynd24p6dszvtchkq7hfkwymi5by5y332wmosy5dwfaqd.onion:119"
	torProxy       = "127.0.0.1:9050"
	maxArticleSize = 32 * 1024 // 32 KB
)

func main() {
	if err := processAndSendRawArticle(os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func processAndSendRawArticle(reader io.Reader) error {
	var rawArticle strings.Builder
	var articleSize int

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		articleSize += len(line) + 1 // +1 for newline character

		if articleSize > maxArticleSize {
			return fmt.Errorf("article size exceeds %d KB", maxArticleSize/1024)
		}
		rawArticle.WriteString(line)
		rawArticle.WriteString("\r\n") // Preserve original line endings
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("error reading input: %v", err)
	}

	return sendRawArticle(rawArticle.String())
}

func sendRawArticle(rawArticle string) error {
	dialer, err := proxy.SOCKS5("tcp", torProxy, nil, proxy.Direct)
	if err != nil {
		return fmt.Errorf("error creating SOCKS5 dialer: %v", err)
	}

	conn, err := dialer.Dial("tcp", server)
	if err != nil {
		return fmt.Errorf("error connecting to the server through Tor: %v", err)
	}
	defer conn.Close()

	writer := &normalizedWriter{conn: conn}
	bufReader := bufio.NewReader(conn)

	serverGreeting, err := bufReader.ReadString('\n') // Read server's greeting
	if err != nil {
		return fmt.Errorf("error reading server greeting: %v", err)
	}
	fmt.Print(serverGreeting) // Optional: Print server greeting for debugging

	// Extract Message-ID (assuming it's in the raw article)
	var messageID string
	scanner := bufio.NewScanner(strings.NewReader(rawArticle))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Message-ID:") || strings.HasPrefix(line, "Message-Id:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				messageID = strings.TrimSpace(parts[1])
				break
			}
		}
	}
	if messageID == "" {
		return fmt.Errorf("could not find Message-ID in the raw article")
	}

	// Send IHAVE command
	fmt.Fprintf(writer, "IHAVE %s\r\n", messageID)
	response, err := bufReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error sending IHAVE command: %v", err)
	}
	fmt.Print(response) // Optional: Print server response for debugging

	if strings.HasPrefix(response, "335") {
		// Send the raw article
		fmt.Fprint(writer, rawArticle)
		fmt.Fprint(writer, ".\r\n")
		response, err = bufReader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("error sending raw article: %v", err)
		}
		fmt.Print(response) // Optional: Print server response for debugging

		if !strings.HasPrefix(response, "235") {
			return fmt.Errorf("article transfer failed: %s", response)
		}
	} else if !strings.HasPrefix(response, "435") {
		return fmt.Errorf("server does not want article: %s", response)
	}

	// Send QUIT command
	fmt.Fprint(writer, "QUIT\r\n")
	return nil
}

type normalizedWriter struct {
	conn io.Writer
}

func (w *normalizedWriter) Write(p []byte) (n int, err error) {
	text := string(p)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n", "\r\n")
	return w.conn.Write([]byte(text))
}