import random,time,hashlib,socket

# generate random lat/lon position
lat = random.uniform(-90,90)
lon = random.uniform(-180,180)
# lat = 5.05
# lon = 4.04
print(lat,lon)

# get current time in millis since epoch
t = time.time()
print(time.localtime(t))
t = int(t*1000)
# print(bin(t))

# get message
def toM(l):
    r = 0
    if l < 0:
        r = 1
        l *= -1
    r = (r << 10) | int(l)
    l = (l-int(l)) * 100000
    r = (r << 17) | int(l)
    print(bin(r))
    return r
m = (((toM(lat) << 8*4) | toM(lon)) << 8*8) | t
print(bin(m))

m = m.to_bytes(16,'big')

# get checksum
c = hashlib.md5(m)

print(c.digest())

packet = m + c.digest()
# packet = b"".join([m, c.digest()])

print(packet)

sock =  socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
res = sock.sendto(packet, ("127.0.0.1",69))

print(res) # i think num bytes sent
