# TODO V0 -> V1

import random,time,ctypes,hashlib,secrets,requests,socket
from multiprocessing.dummy import Pool as ThreadPool
from Crypto.Cipher import AES

def random_packet(i,p=False):
    # generate random lat/lon position
    lat = random.uniform(-90,90)
    lon = random.uniform(-180,180)
    # lat = 5.05
    # lon = 4.04
    if p: print(lat,lon)

    # get current time in millis since epoch
    # t = time.time()
    # t = int(t*1000)
    t = int(i)
    if p: print(t)
    # print(bin(t))

    # get message
    def toM(l):
        # r = 0
        # if l < 0:
        #     r = 1
        #     l *= -1
        # r = (r << 10) | int(l)
        # l = (l-int(l)) * 100000
        # r = (r << 17) | int(l)
        # if p: print(bin(r))
        # return r
        return ctypes.c_uint.from_buffer(ctypes.c_float(l)).value.to_bytes(4, 'big')
    
    # m = (((toM(lat) << 8*4) | toM(lon)) << 8*8) | t
    # if p: print(bin(m))
    # m = m.to_bytes(16,'big')
    m = toM(lat) + toM(lon) + t.to_bytes(8, 'big')

    # get checksum
    c = hashlib.md5(m)

    if p: print(c.digest())

    packet = m + c.digest()
    # packet = b"".join([m, c.digest()])

    if p: print(packet)
    return (packet, {"time":t, "latitude":lat, "longitude":lon})

# num_conn = 3
# num_packets = 10
num_conn = 1
num_packets = 5000

# def transmit(key, packet=None):
#     # get data packet
#     if packet is None: packet = random_packet()
#     # encrypt packet using AES-256
#     packet = AES.new(bytearray.fromhex(key), AES.MODE_ECB).encrypt(packet)

#     sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
#     res = sock.sendto(packet, ("127.0.0.1",69))

#     print(res) # i think num bytes sent
#     return

def doit(me):
    # encryption key, start session
    key = secrets.token_hex(nbytes=32)
    r = requests.post('http://127.0.0.1:1025/session/start', data="""{
        \"username\":\"test\",
        \"password\":\"test\",
        \"key\":\""""+key+"""\"
    }""")
    tmp = r.json()
    print(me, tmp)
    port = tmp["port"]
    end = tmp["token"]
    # create socket
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    #sock = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
    # transmit random data packets
    packets = []
    for i in range(num_packets): # generate packets first
        (packet, obj) = random_packet(i, p=False)
        packets.append((obj, AES.new(bytearray.fromhex(key), AES.MODE_ECB).encrypt(packet)))
    startTime = time.time()
    for i in range(num_packets):
        # send it
        sock.sendto(packets[i][1], ("127.0.0.1",port)) # print?
        #sock.sendto(packet, "/tmp/{}.sock".format(port))
        # wait
        # time.sleep(random.randint(10,1000)/1000)
    elapsedTime = time.time() - startTime
    # we're done
    # time.sleep(0.5)
    r = requests.post('http://127.0.0.1:1025/session/stop', data="{\"port\":"+str(port)+",\"token\":\""+end+"\"}")
    text = r.text
    time.sleep(1)
    r = requests.get('http://127.0.0.1:1025/log/'+end)
    # print(r.text)
    log = r.json()
    # return (packets, r.text)
    success = num_packets == len(log["data"])
    if success:
        for i in range(num_packets):
            p = packets[i][0]
            q = log["data"][i]
            success = success and (p["time"]==q["time"] and abs(p["latitude"] - q["latitude"]) < 0.0001 and abs(p["longitude"] - q["longitude"]) < 0.0001)
    return (text, len(log["data"]), success, elapsedTime, num_packets/elapsedTime)

if num_conn > 1:
    pool = ThreadPool(num_conn)
    results = pool.map(doit, list(range(num_conn)))
    pool.close()
else:
    results = [doit(0)]

for i in range(num_conn):
    print(results[i])
