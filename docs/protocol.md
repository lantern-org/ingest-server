# Protocol Specs

Details the UDP packet specification

## V1

```txt
infoxxxx 00000000 00000000 00000000 00000000 more info
============================================
misc     b000bbbb 00000000 uuuuuuuu uuuuuuuu (1)
index    uuuuuuuu uuuuuuuu uuuuuuuu uuuuuuuu (2)
time     bbbbbbbb bbbbbbbb bbbbbbbb bbbbbbbb (3)
         bbbbbbbb bbbbbbbb bbbbbbbb bbbbbbbb
--------------------------------------------
lat      ffffffff ffffffff ffffffff ffffffff IEEE 754 float
lon      ffffffff ffffffff ffffffff ffffffff
accuracy ffffffff ffffffff ffffffff ffffffff
         00000000 00000000 00000000 00000000 reserved
--------------------------------------------
sum      cccccccc cccccccc cccccccc cccccccc (4)
         cccccccc cccccccc cccccccc cccccccc
         cccccccc cccccccc cccccccc cccccccc
         cccccccc cccccccc cccccccc cccccccc
============================================
used = 3 blocks * 16 bytes/block = 48 bytes
256 MAX BYTES -> 48/256 = 18.75% utilization

notes
0. all integer-like values are BIG-endian
1. misc info
   - `b[ 0 ,  0]=0,1` -- 0=BigEndian, 1=LittleEndian -- endianness of data in packet
   - `b[ 1 ,  3]=0` reserved
   - `b[ 4 ,  7]=0-4` min-max unsigned half-byte (space for `[0,15]`) -- internet signal strength
   - `b[ 8 , 15]=0` reserved
   - `b[16 , 31]=0-65535` unsigned 2-byte integer -- packet protocol version
2. unsigned 4-byte int -- packet number
   - if we send one packet per millisecond, then the max recording time is 2^32 / 1000 / 60 / 60 / 24 ~ 49.7 days
      - the maximum amount of transmitted data is thus 2^32 * 48 / 1024 / 1024 / 1024 = 192 GB
   - useful for ordering, avoiding replay attacks
3. ms since epoch
   - the universe will die before we run out of milliseconds since 01/01/1970
   - useful for ordering, avoiding replay attacks
4. MD5 checksum
   - MD5 isn't as strong as SHA256, but it's half the space
   - plus, we're only using for corruption-checking, not security
   - we _could_ consider selecting a subset of bytes and transmitting just those

 packet  = data without checksum
transmit = AES-256-ECB([packet, MD5(packet)])
```

## V0

```txt
lat  0000siii iiiiiiif ffffffff ffffffff (-> use IEEE 754 float instead)
lon  0000siii iiiiiiif ffffffff ffffffff
time bbbbbbbb bbbbbbbb bbbbbbbb bbbbbbbb (useful for ordering, avoiding replay attacks)
     bbbbbbbb bbbbbbbb bbbbbbbb bbbbbbbb (required: BIG-endian)
sum  cccccccc cccccccc cccccccc cccccccc (MD5 isn't as strong as SHA256)
     cccccccc cccccccc cccccccc cccccccc (but we're only using for corruption-checking)
     cccccccc cccccccc cccccccc cccccccc (consider selecting a few bytes and transmitting just those?)
     cccccccc cccccccc cccccccc cccccccc
```
