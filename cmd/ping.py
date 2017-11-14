import requests

proxies = {'http': 'socks5://127.0.0.1:51959', 'https': 'socks5://127.0.0.1:51959'}

r = requests.get('http://httpbin.org/ip', proxies=proxies)
print('.', 'host of daze-server is', r.json()['origin'])

r = requests.post('http://httpbin.org/post', data='data', proxies=proxies)
print('.', r.json()['data'])

r = requests.get('https://httpbin.org/ip', proxies=proxies)
print('.', 'host of daze-server is', r.json()['origin'])

r = requests.post('https://httpbin.org/post', data='data', proxies=proxies)
print('.', r.json()['data'])

proxies = {'http': 'http://127.0.0.1:51959', 'https': 'https://127.0.0.1:51959'}

r = requests.get('http://httpbin.org/ip', proxies=proxies)
print('.', 'host of daze-server is', r.json()['origin'])

r = requests.post('http://httpbin.org/post', data='data', proxies=proxies)
print('.', r.json()['data'])

r = requests.get('https://httpbin.org/ip', proxies=proxies)
print('.', 'host of daze-server is', r.json()['origin'])

r = requests.post('https://httpbin.org/post', data='data', proxies=proxies)
print('.', r.json()['data'])
