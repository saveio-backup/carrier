/*
Current limitations:

	- GSS-API authentication is not supported
	- only SOCKS version 5 is supported
	- TCP bind and UDP not yet supported

Example http client over SOCKS5:

	proxy := &socks.Proxy{"127.0.0.1:1080"}
	tr := &http.Transport{
		Dial: proxy.Dial,
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get("https://example.com")
*/
package udp

