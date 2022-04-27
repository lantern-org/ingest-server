# System Flow

(assuming 100% success)

```txt

user[phone[gps,internet]]                       ingest-server
                          ---------(1)--------->

                         <---------(2)---------

                          ---------(3)--------->
                               ---.....-->

friend[internet]
                          ---------(4)--------->
                         <---------(5)---------
                                  .....

user[phone[gps,internet]]
                          ---------(6)--------->

                         <---------(7)---------

(1) [POST] https://ADDRESS/session/start
{
    "username": string, // username for the server
    "password": string, // password for the username
    "key":      string  // 32 bytes encoded to hexadecimal string
}

(2) 200 response
{
    "port":  int,    // port to send UDP packets
    "token": string, // token to give back to the server for stopping the session
    "code":  string  // 4-character read-only token
}

(3) [UPD] ADDRESS:port
packet[bytes]

(4) [GET] https://ADDRESS/location/{code}
uses the code from (2)

(5) response (may depend on the packet protocol version)
{
    "version":   uint32  // packet protocol version
	"index":     uint32  // packet index
	"time":      int64   // unix epoch
	"latitude":  float32 // degrees
	"longitude": float32 // degrees
	"accuracy":  float32 // radius meters
	"internet":  byte    // 0-4 (space for 0-15)
    "status":    string  // last-known status update from user
}

(6) [POST] https://ADDRESS/session/stop
{
    "port":  int,   // port from (2)
    "token": string // token from (2)
}

(7) 200 response
{
    "success": boolean // true
}
```
