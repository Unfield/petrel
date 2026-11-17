#!/bin/sh
# Starts Stalwart in Docker as a JMAP (and IMAP, POP3, SMTP) server for
# Petrel's tests, with users sam@petrel.test and lena@petrel.test, password
# "petrel-jmap-test". Then:
#
#   PETREL_JMAP_URL=http://127.0.0.1/.well-known/jmap \
#   PETREL_JMAP_USER1=sam@petrel.test PETREL_JMAP_USER2=lena@petrel.test \
#   PETREL_JMAP_PASSWORD=petrel-jmap-test go test ./internal/mail -run JMAP
#
# PETREL_REAL_IMAP=127.0.0.1:2993 PETREL_REAL_POP3=127.0.0.1:2995 PETREL_REAL_SMTP=127.0.0.1:2465
# also test IMAP, POP3 and SMTP over TLS against it (TestRealServers).
# Sam has a 100 MB quota, for the storage meter.
#
# Remove it with: docker rm -f petrel-stalwart
set -eu
NAME=petrel-stalwart
PASS=petrel-jmap-test
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" -p 80:8080 -p 8080:8080 -p 2143:143 -p 2993:993 -p 2995:995 -p 2465:465 -p 2587:587 -p 2110:110 -p 2025:25 stalwartlabs/stalwart:latest >/dev/null
until curl -s -o /dev/null http://127.0.0.1:8080/; do sleep 1; done
sleep 1
BOOT=$(docker logs "$NAME" 2>&1 | sed -n 's/^ *password: *//p' | head -1)

jmap() { # user:password, method calls
	curl -s -u "$1" -H 'Content-Type: application/json' http://127.0.0.1:8080/jmap/ \
		-d "{\"using\":[\"urn:ietf:params:jmap:core\",\"urn:stalwart:jmap\"],\"methodCalls\":$2}"
}

# The first setup makes the domain and a permanent administrator.
ADMIN=$(jmap "admin:$BOOT" '[["x:Bootstrap/set",{"accountId":"d333333","update":{"singleton":{"defaultHostname":"mail.petrel.test","defaultDomain":"petrel.test","requestTlsCertificate":false,"generateDkimKeys":false}}},"0"]]' |
	python3 -c 'import json,sys; print(json.load(sys.stdin)["methodResponses"][0][1]["updated"]["singleton"]["secret"])')
docker restart "$NAME" >/dev/null
until curl -s -o /dev/null http://127.0.0.1:8080/; do sleep 1; done
sleep 1
A="admin@petrel.test:$ADMIN"
# Announce JMAP as http://127.0.0.1 (port 80 maps to the container).
jmap "$A" '[["x:SystemSettings/set",{"accountId":"b","update":{"singleton":{"services/jmap":{"hostname":"127.0.0.1","cleartext":true}}}},"0"]]' |
	grep -q '"updated"' || { echo "couldn't set the JMAP address" >&2; exit 1; }
jmap "$A" "[[\"x:Account/set\",{\"accountId\":\"b\",\"create\":{
	\"sam\":{\"@type\":\"User\",\"name\":\"sam\",\"quotas\":{\"maxDiskQuota\":104857600},\"domainId\":\"b\",\"description\":\"Sam Taylor\",\"credentials\":{\"0\":{\"@type\":\"Password\",\"secret\":\"$PASS\"}},\"roles\":{\"@type\":\"User\"}},
	\"lena\":{\"@type\":\"User\",\"name\":\"lena\",\"domainId\":\"b\",\"description\":\"Lena Fischer\",\"credentials\":{\"0\":{\"@type\":\"Password\",\"secret\":\"$PASS\"}},\"roles\":{\"@type\":\"User\"}}}},\"0\"]]" >/dev/null
echo "Administrator: admin@petrel.test / $ADMIN (http://127.0.0.1:8080/admin)"
docker restart "$NAME" >/dev/null # applies the JMAP address
until curl -s -o /dev/null http://127.0.0.1:8080/; do sleep 1; done
sleep 1
curl -s -L -u "sam@petrel.test:$PASS" http://127.0.0.1:8080/.well-known/jmap | python3 -c 'import json,sys; s=json.load(sys.stdin); print("JMAP ready:", s["username"], s["apiUrl"])'
