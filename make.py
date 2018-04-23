import os
import os.path
import subprocess
import sys

project_name = os.path.basename(os.path.dirname(os.path.abspath(__file__)))
gopath = os.getenv('GOPATH')
if gopath:
    gopath = gopath.split(os.pathsep)[0]
else:
    gopath = os.path.expanduser('~/go')


def call(command):
    print(command)
    r = subprocess.call(command, shell=True)
    if r != 0:
        sys.exit(r)


def link():
    src = os.getcwd()
    dst = os.path.normpath(os.path.join(gopath, 'src', 'github.com', 'mohanson', project_name))
    if os.path.exists(dst):
        return
    os.makedirs(os.path.dirname(dst), exist_ok=True)
    if os.name == 'nt':
        call(f'mklink /J {dst} {src}')
    else:
        call(f'ln -s -f {src} {dst}')


def main():
    call(f'go install github.com/mohanson/{project_name}')
    call(f'go install github.com/mohanson/{project_name}/protocol/ashe')
    call(f'go install github.com/mohanson/{project_name}/protocol/asheshadow')
    call(f'go install github.com/mohanson/{project_name}/cmd/{project_name}')


if __name__ == '__main__':
    main()
