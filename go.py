import subprocess
import sys


def call(command):
    print(command)
    r = subprocess.call(command, shell=True)
    if r != 0:
        sys.exit(r)


def main():
    call('go install github.com/mohanson/daze/cmd/daze')


if __name__ == '__main__':
    main()
