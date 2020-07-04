# pip3 install PySocks
import sys
import socket
import socks

s = socks.socksocket(socket.AF_INET, socket.SOCK_DGRAM)
s.set_proxy(socks.SOCKS5, "127.0.0.1", 1080)
s.sendto(b"Hello World!\n", ("127.0.0.1", 8080))
sys.stdout.write(s.recv(bufsize=13).decode())
