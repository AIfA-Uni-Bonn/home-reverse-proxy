package main

import (
	"fmt"
	"log"

	"github.com/go-ldap/ldap/v3"
)

func main() {
	l, err := ldap.DialURL("ldaps://ldap2.astro.uni-bonn.de")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	searchRequest := ldap.NewSearchRequest(
		"ou=People,dc=astro,dc=uni-bonn,dc=de", // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(uid=ocordes)",
		//"(&(objectClass=organizationalPerson))", // The filter to apply
		[]string{"dn", "cn", "authorizedService", "homeDirectory"}, // A list attributes to retrieve
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range sr.Entries {
		fmt.Printf("%s: %v, %v, %v\n", entry.DN, entry.GetAttributeValue("cn"),
			entry.GetAttributeValues("authorizedService"),
			entry.GetAttributeValue(("homeDirectory")))
	}
}
