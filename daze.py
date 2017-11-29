"""usage: daze <command> [<args>]

The most commonly used daze commands are:
  server     Start daze server
  client     Start daze client
  cmd        Execute a command by a running client

Run 'daze <command> -h' for more information on a command."""

import argparse
import concurrent.futures
import datetime
import ipaddress
import math
import os
import os.path
import random
import socket
import struct
import subprocess
import sys

import dns.resolver
import requests


class KeySizeError(Exception):
    pass


class Cipher:
    def __init__(self, key):
        assert isinstance(key, bytes)
        self.s = list(range(256))
        self.i = 0
        self.j = 0
        self.key = key

        k = len(key)
        if k < 1 or k > 256:
            raise KeySizeError('crypto/rc4: invalid key size ' + str(k))

        j = 0
        for i in range(256):
            j = (j + self.s[i] + key[i % k]) % 256
            self.s[i], self.s[j] = self.s[j], self.s[i]

    def __str__(self):
        return f'rc4.Cipher(key={self.key})'

    def xor_key_stream_generic(self, src):
        dst = list(range(len(src)))
        i, j = self.i, self.j
        for k, v in enumerate(src):
            i = (i + 1) % 256
            j = (j + self.s[i] % 256) % 256
            self.s[i], self.s[j] = self.s[j], self.s[i]
            dst[k] = v ^ self.s[(self.s[i] + self.s[j]) % 256] % 256
        self.i, self.j = i, j
        return bytes(dst)


class GravityDazeConn:
    def __init__(self, socket, k):
        self.socket = socket
        self.wc = Cipher(k)
        self.rc = Cipher(k)
        self.close = self.socket.close

    def send(self, data):
        data = self.wc.xor_key_stream_generic(data)
        self.socket.sendall(data)

    def recv(self, size):
        data = self.socket.recv(size)
        data = self.rc.xor_key_stream_generic(data)
        return data


class DazeError(Exception):
    pass


def copy(src, dst):
    while True:
        try:
            data = src.recv(32 * 1024)
        except Exception:
            break
        if not data:
            break
        try:
            dst.send(data)
        except Exception:
            break
    src.close()
    dst.close()


def socket_daze():
    sock = socket.socket()
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    return sock


class Server:
    def __init__(self, listen_host, listen_port):
        self.listen_host = listen_host
        self.listen_port = listen_port
        self.executor = concurrent.futures.ThreadPoolExecutor(128)

    def _serve(self, sock):
        sock.settimeout(10)
        data = sock.recv(128)
        conn = GravityDazeConn(sock, data)
        data = conn.recv(128)
        if data[0] != 0xFF or data[1] != 0xFF:
            raise DazeError(f'malformed request: {list(data[:2])}')
        d = int.from_bytes(data[120:128], 'big')
        if abs(datetime.datetime.now().timestamp() - d) > 60:
            d_str = datetime.datetime.fromtimestamp(d).strftime('%Y-%m-%d %H:%M:%S')
            raise DazeError(f'expired: {d_str}')
        data = conn.recv(2 + 256)
        dest = data[2:2 + int(data[1])].decode()
        seps = dest.split(':')
        host = seps[0]
        port = int(seps[1])
        dest = socket_daze()
        print(f'connect {host}:{port}')
        try:
            dest.connect((host, port))
        except Exception:
            dest.close()
            raise
        sock.settimeout(None)
        self.executor.submit(copy, conn, dest)
        copy(dest, conn)

    def serve(self, sock):
        try:
            self._serve(sock)
        except Exception as e:
            print(e)
        sock.close()

    def run(self):
        s = socket_daze()
        s.bind((self.listen_host, self.listen_port))
        print(f'serve on {self.listen_host}:{self.listen_port}')
        s.listen()
        while True:
            sock, _ = s.accept()
            self.executor.submit(self.serve, sock)


class Client:
    def __init__(self, server_host, server_port):
        self.server_host = server_host
        self.server_port = server_port

    def connect(self, host, port):
        s = socket_daze()
        s.connect((self.server_host, self.server_port))
        k = bytes(random.randint(0, 255) for _ in range(128))
        try:
            s.sendall(k)
        except Exception:
            s.close()
            raise

        conn = GravityDazeConn(s, k)
        try:
            data = [random.randint(0, 255) for _ in range(386)]
            data[0] = 255
            data[1] = 255
            time = list(struct.pack('>I', int(datetime.datetime.now().timestamp())).rjust(8, chr(0).encode()))
            data[120:128] = time
            data[128] = 1
            address = f'{host}:{port}'
            data[129] = len(address)
            data[130:130 + len(address)] = [ord(c) for c in address]
            conn.send(data)
        except Exception:
            conn.close()
            raise
        return conn


class Locale:
    def __init__(self, listen_host, listen_port, dialer):
        self.listen_host = listen_host
        self.listen_port = listen_port
        self.dialer = dialer
        self.executor = concurrent.futures.ThreadPoolExecutor(128)

    def _serve(self, sock):
        data = sock.recv(2)
        n = int(data[1])
        sock.recv(n)
        sock.send(bytes([0x05, 0x00]))
        data = sock.recv(4)
        host = ''
        if data[3] == 1:
            data = sock.recv(4)
            host = ipaddress.IPv4Address(data).compressed
        elif data[3] == 3:
            data = sock.recv(1)
            data = sock.recv(int(data[0]))
            host = data.decode()
        elif data[3] == 4:
            data = sock.recv(16)
            host = ipaddress.IPv6Address(data).compressed
        else:
            raise DazeError('unsupported dst')
        data = sock.recv(2)
        port = data[0] * 256 + data[1]
        try:
            dest = self.dialer.connect(host, port)
        except Exception:
            sock.send(bytes([0x05, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]))
            raise
        else:
            sock.send(bytes([0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]))

        self.executor.submit(copy, sock, dest)
        copy(dest, sock)

    def serve(self, sock):
        try:
            self._serve(sock)
        except Exception as e:
            print(e)
        sock.close()

    def run(self):
        s = socket_daze()
        s.bind((self.listen_host, self.listen_port))
        print(f'serve on {self.listen_host}:{self.listen_port}')
        s.listen()
        while True:
            sock, _ = s.accept()
            self.executor.submit(self.serve, sock)


class Dialer:
    def __init__(self, client):
        self.client = client
        self.cached_file = os.path.join(os.path.expanduser('~'), '.daze', 'delegated-apnic-latest')
        self.url = 'http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest'
        self.cidr_list = []
        self.res = dns.resolver.Resolver()
        self.res.nameservers = ['8.8.8.8', '8.8.4.4']

        if not os.path.exists(self.cached_file) or \
                (datetime.datetime.now().timestamp() - os.path.getmtime(self.cached_file)) > 3600 * 24 * 28:
            print('update', self.cached_file)
            with open(self.cached_file, 'wb') as f:
                f.write(requests.get(self.url).content)

        with open(self.cached_file, 'r') as f:
            for line in f:
                line = line.rstrip()
                if not line.startswith('apnic|CN|ipv4'):
                    continue
                seps = line.split('|')
                sep4 = int(seps[4])
                mask = 32 - int(math.log2(sep4))
                cidr = ipaddress.IPv4Network(f'{seps[3]}/{mask}')
                self.cidr_list.append(cidr)

    def surmise(self, host):
        try:
            ip = ipaddress.ip_address(host)
        except ValueError:
            answer = self.res.query(host)
            if not answer:
                return 2
            ip = ipaddress.ip_address(answer[0])
        if ip.is_private:
            return 0
        for cidr in self.cidr_list:
            if ip in cidr:
                return 0
        return 1

    def connect(self, host, port):
        road = self.surmise(host)
        print('connect', road, f'{host}:{port}')
        if road == 0:
            s = socket_daze()
            s.connect((host, port))
            return s
        elif road == 1:
            return self.client.connect(host, port)
        elif road == 2:
            try:
                s = socket_daze()
                s.connect((host, port))
                return s
            except Exception:
                return self.client.connect(host, port)
        else:
            pass


def main(args=None):
    args = sys.argv[1:] if not args else args
    if not args:
        print(__doc__)
        return
    if args[0] == 'server':
        parser = argparse.ArgumentParser()
        parser.add_argument('-l', '--listen', default='0.0.0.0:51958', help='listen address')
        args = parser.parse_args(args[1:])
        seps = args.listen.split(':')
        listen_host = seps[0]
        listen_port = int(seps[1])
        server = Server(listen_host, listen_port)
        server.run()
    elif args[0] == 'client':
        parser = argparse.ArgumentParser()
        parser.add_argument('-l', '--listen', default='127.0.0.1:51959', help='listen address')
        parser.add_argument('-s', '--server', default='127.0.0.1:51958', help='server address')
        args = parser.parse_args(args[1:])
        seps = args.server.split(':')
        server_host = seps[0]
        server_port = int(seps[1])
        seps = args.listen.split(':')
        listen_host = seps[0]
        listen_port = int(seps[1])
        c = Client(server_host, server_port)
        d = Dialer(c)
        l = Locale(listen_host, listen_port, d)
        l.run()
    elif args[0] == 'cmd':
        parser = argparse.ArgumentParser()
        parser.add_argument('-c', '--client', default='127.0.0.1:51959', help='client address')
        parser.add_argument('cmd', nargs=argparse.REMAINDER)
        args = parser.parse_args(args[1:])
        env = os.environ.copy()
        env['all_proxy'] = args.client
        subprocess.call(' '.join(args.cmd), shell=True, env=env)
    else:
        print(__doc__)
        return


if __name__ == '__main__':
    main()
