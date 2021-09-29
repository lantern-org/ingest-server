import random,time,hashlib,secrets,requests,socket
from multiprocessing.dummy import Pool as ThreadPool
from Crypto.Cipher import AES

def random_packet(p=False):
    # generate random lat/lon position
    lat = random.uniform(-90,90)
    lon = random.uniform(-180,180)
    # lat = 5.05
    # lon = 4.04
    if p: print(lat,lon)

    # get current time in millis since epoch
    t = time.time()
    if p: print(time.localtime(t))
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
        if p: print(bin(r))
        return r
    m = (((toM(lat) << 8*4) | toM(lon)) << 8*8) | t
    if p: print(bin(m))

    m = m.to_bytes(16,'big')

    # get checksum
    c = hashlib.md5(m)

    if p: print(c.digest())

    packet = m + c.digest()
    # packet = b"".join([m, c.digest()])

    if p: print(packet)
    return packet

num_conn = 3
num_packets = 10

def transmit(key, packet=None):
    # get data packet
    if packet is None: packet = random_packet()
    # encrypt packet using AES-256
    packet = AES.new(bytearray.fromhex(key), AES.MODE_ECB).encrypt(packet)

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    res = sock.sendto(packet, ("127.0.0.1",69))

    print(res) # i think num bytes sent
    return

def doit(me):
    # encryption key, start session
    key = secrets.token_hex(nbytes=32)
    r = requests.post('http://127.0.0.1:420/session/start', data="{\"key\":\""+key+"\"}")
    tmp = r.json()
    print(me, tmp)
    port = tmp["port"]
    end = tmp["token"]
    # create socket
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    # transmit random data packets
    packets = []
    for i in range(num_packets):
        packet = random_packet()
        packets.append(packet)
        # encrypt random packet using AES-256
        packet = AES.new(bytearray.fromhex(key), AES.MODE_ECB).encrypt(packet)
        # send it
        sock.sendto(packet, ("127.0.0.1",port)) # print?
        # wait
        time.sleep(random.randint(10,1000)/1000)
    # we're done
    r = requests.post('http://127.0.0.1:420/session/end', data="{\"port\":"+str(port)+",\"token\":\""+end+"\"}")
    return (packets, r.text)

pool = ThreadPool(num_conn)
results = pool.map(doit, list(range(num_conn)))

for i in range(num_conn):
    print(results[i])
