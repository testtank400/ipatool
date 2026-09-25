package appstore

import (
	"net/url"
	"strings"
)

// ensureAccountPod fills Account.Pod from the itspod cookie when login did not
// persist a pod. Download/list-versions hit p{N}-buy.itunes.apple.com; without
// the pod prefix the session cookies for that host are not sent and Apple
// often answers with an empty songList.
func (t *appstore) ensureAccountPod(acc Account) Account {
	if acc.Pod != "" || t.cookieJar == nil {
		return acc
	}

	for _, host := range []string{
		"buy.itunes.apple.com",
		"itunes.apple.com",
		"apple.com",
	} {
		u := &url.URL{Scheme: "https", Host: host, Path: "/"}
		for _, c := range t.cookieJar.Cookies(u) {
			if strings.EqualFold(c.Name, "itspod") && c.Value != "" {
				acc.Pod = c.Value
				return acc
			}
		}
	}

	return acc
}
