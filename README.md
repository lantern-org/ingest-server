# ingest-server

You MUST host this server behind an HTTPS proxy!

## helplful links

<https://stackoverflow.com/questions/40917372/udp-forwarding-with-nginx>

<https://stackoverflow.com/questions/2974987/options-for-securing-udp-traffic>
<http://nginx.org/patches/dtls/README.txt>

<https://gist.github.com/miekg/d9bc045c89578f3cc66a214488e68227>
<https://ops.tips/blog/udp-client-and-server-in-go/#receiving-from-a-udp-connection-in-a-server>

## encrypting the channel

We assume we have a secure connection over HTTPS via TLS.
To start a session, we generate a 32-byte (256-bit) key (basically a really long password) on the phone-side.
Then we share this key to the server over a TLS secure connection.
We encrypt the(all) UDP packet(s) on the phone-side using the key with AES-256.
We send the packet(s) to the server on an insecure line, and decrypt using the shared key.
Done.

## packet construction

On the phone-end, we have to get the lat/lon and send to the server.
On Android, lat/lon are returned as a `double`.
We assume 3-digit integers with 5-digit fractional parts.
For one value, that's 8 digits.

If we convert each digit to a character, then encode into binary, that's 8 bytes per value.
Obviously we want to limit our sent-data as much as possible.
I don't really want to do huffman encoding or any sort of compression, though (but we could, for sure).
Okay, well 8 digits in binary is just `log_2(100000000) = 8/log_10(2) ~ 26.58`, so 4 bytes (maybe 3 depending on what value we actually would go up to).
We've already halved our byte value, lovely.
I'm sure there are better ways to translate GPS to binary.

Of course, we still have negatives to worry about.
Maybe we should format a bit-string.
1 bit for sign, 10 bits for integer part, 17 bits for decimal part
```
0000siii iiiiiiif ffffffff ffffffff
```
Honestly good enough.

Other data to send:
- date/time of recorded position (Location.getTime returns long == 64 bits == 8 bytes)
- checksum (SHA-256 returns 256 bits = 32 bytes -- MD5 returns 128 bits = 16 bytes)

```
lat  0000siii iiiiiiif ffffffff ffffffff (-> use IEEE 754 float instead)
lon  0000siii iiiiiiif ffffffff ffffffff
time bbbbbbbb bbbbbbbb bbbbbbbb bbbbbbbb (useful for ordering, avoiding replay attacks)
     bbbbbbbb bbbbbbbb bbbbbbbb bbbbbbbb (required: BIG-endian)
sum  cccccccc cccccccc cccccccc cccccccc (MD5 isn't as strong as SHA256)
     cccccccc cccccccc cccccccc cccccccc (but we're only using for corruption-checking)
     cccccccc cccccccc cccccccc cccccccc (consider selecting a few bytes and transmitting just those?)
     cccccccc cccccccc cccccccc cccccccc
```
totals 32 byte packet -- that's pretty good

latitude runs -90 to +90
longitude runs -180 to +180

## multiple clients

if someone else wants to send data, then we give them a new port

ideally, we'll run a server on multiple nodes on the same network, where each node/server serves a port range decided by the orchestrator in a command-line arg

and then the overall network mapper can send a range of ports all to the local network

## user authentication

barebones.
```
database = [
     {"username":string, "password":string, "salt":string},
     ...
]
```
we have a python script that manages the password database (for now).
basic crypto.
client sends plaintext username/password pair (over HTTPS!).
server looks up username, gets the salt, hashes `password+salt`, checks hash against stored hash.
use SHA256 hash.
command-line flag to specify password database file.
should vet the file as well.

to make it slightly easier, we'll take advantage of the `bcrypt` library.
basically, this handles the salting automatically.
it does this simply by generating a random salt upon password generation, doing the `bcrypt` algo, and appending the salt to the front of the resulting hash.
then, to check a plaintext password, the library just pulls out the salt from the hashed password and uses that to hash the plaintext (and checks the result for a match).
basically it makes my life slightly easier.
```
database = [
     {"username":string, "password":string},
     ...
]
```

needs to be in `database.json` file
