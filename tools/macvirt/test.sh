#!/bin/sh
swift build -c release --disable-sandbox
codesign -s - --entitlements macvirt.entitlements .build/release/macvirt
chmod +x .build/release/macvirt
.build/release/macvirt --memory 512 --cpu-count 2 --kernel-path /Users/tobi/Desktop/vm/vmlinux --initrd-path /Users/tobi/Desktop/vm/initrd --disk-path /Users/tobi/Desktop/vm/disk.img --cmd-line-arg "console=hvc0 irqfixup root=/dev/vda"