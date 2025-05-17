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
	server         = "news.tcpreset.net:119"
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
		articleSize += len(line) + 1

		if articleSize > maxArticleSize {
			return fmt.Errorf("article size exceeds %d KB", maxArticleSize/1024)
		}
		rawArticle.WriteString(line)
		rawArticle.WriteString("\r\n")
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

	_ , err = bufReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading server greeting: %v", err)
	}

	fmt.Fprint(writer, "POST\r\n")
	response, err := bufReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error sending POST command: %v", err)
	}

	if strings.HasPrefix(response, "340") { // Typischerweise Antwort auf POST
		// Senden des rohen Artikels
		fmt.Fprint(writer, rawArticle)
		fmt.Fprint(writer, ".\r\n")
		response, err = bufReader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("error sending raw article: %v", err)
		}
		fmt.Print(response) // Optional: Print server response for debugging

		if !strings.HasPrefix(response, "240") { // Typische Erfolgsantwort auf POST
			return fmt.Errorf("article transfer failed: %s", response)
		}
	} else {
		return fmt.Errorf("server did not accept POST command: %s", response)
	}

	// Senden des QUIT-Befehls
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