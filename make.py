import os
import os.path
import subprocess

project_name = os.path.basename(os.path.dirname(os.path.abspath(__file__)))


def call(command):
    print(command)
    subprocess.call(command, shell=True)


def link():
    src = os.getcwd()
    gopath = os.getenv('GOPATH').split(os.pathsep)[0]
    dst = os.path.normpath(os.path.join(gopath, 'src', 'github.com', 'mohanson', project_name))
    if os.path.exists(dst):
        return
    os.makedirs(os.path.dirname(dst), exist_ok=True)
    if os.name == 'nt':
        call(f'mklink /J {dst} {src}')
    else:
        call(f'ln -s -f {src} {dst}')


def main():
    call(f'go install github.com/mohanson/{project_name}/cmd/{project_name}')


if __name__ == '__main__':
    main()
