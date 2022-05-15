# API Specs

Remember, the API **must** be hosted behind HTTPS.
(for now, anyway)

```txt
[meth] /path
<- input
 * notes (optional)
-> output
 * notes (optional)

[GET]  /health (TODO)
<-
-> healthy | unhealthy

[POST] /session/start
<- json
{
    "username":string,
    "password":string,
    "key":string // string should be hex values encoded to characters -- len == 32
}
-> json
{
    "port":int,
    "token":string,
    "code":string
}
OR
{
    "error":string
}

[POST] /session/stop
<- json
{
    "port":int,
    "token":string
}
-> json
{
    "success":true
}
OR
{
    "error":string
}

[GET]  /location/{CODE}
<-
-> json
{
    "version":uint16, "index":uint32, "time":int64,
    "latitude":float32, "longitude":float32, "accuracy":float32,
    "internet":byte "processed":int64 "status":string
}
{"error":string}

[GET]  /log/{TOKEN}
<-
-> json
{ ... }
{"error":string}

[GET]  /(*)
<-
-> $1
```
