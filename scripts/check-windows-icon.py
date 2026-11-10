#!/usr/bin/env python3
"""Fail a Windows build if its embedded icon differs from NetMerge's source ICO.

Uses Python's standard library; verifies PE resource trees and every icon image,
not just the presence of a .rsrc section or an icon-looking byte sequence.
"""
import argparse
from pathlib import Path
import struct


def verify(executable, icon):
    data = executable.read_bytes()
    ico = icon.read_bytes()
    if data[:2] != b'MZ':
        raise ValueError('Not a Windows executable')
    pe = struct.unpack_from('<I', data, 0x3c)[0]
    if data[pe:pe + 4] != b'PE\0\0':
        raise ValueError('Invalid PE signature')
    sections = struct.unpack_from('<H', data, pe + 6)[0]
    optional_size = struct.unpack_from('<H', data, pe + 20)[0]
    optional = pe + 24
    magic = struct.unpack_from('<H', data, optional)[0]
    if magic not in (0x10b, 0x20b):
        raise ValueError('Unsupported PE optional header')
    directories = optional + (112 if magic == 0x20b else 96)
    resource_rva, resource_size = struct.unpack_from('<II', data, directories + 16)
    if not resource_rva or not resource_size:
        raise ValueError('Missing Windows icon resources')

    def offset(rva):
        for i in range(sections):
            p = optional + optional_size + i * 40
            virtual_size, address, raw_size, raw_offset = struct.unpack_from('<IIII', data, p + 8)
            if address <= rva < address + max(virtual_size, raw_size):
                return raw_offset + rva - address
        raise ValueError(f'Unmapped resource address {rva:#x}')

    resource = offset(resource_rva)

    def entries(relative):
        p = resource + relative
        named, ids = struct.unpack_from('<HH', data, p + 12)
        return dict(struct.unpack_from('<II', data, p + 16 + i * 8) for i in range(named + ids))

    def children(entry):
        if not entry & 0x80000000:
            raise ValueError('Expected resource directory')
        return entries(entry & 0x7fffffff)

    def payload(entry):
        languages = children(entry)
        if not languages:
            raise ValueError('Missing resource language')
        leaf = next(iter(languages.values()))
        if leaf & 0x80000000:
            raise ValueError('Unexpected resource directory')
        rva, length = struct.unpack_from('<II', data, resource + leaf)
        p = offset(rva)
        return data[p:p + length]

    root = entries(0)
    if 3 not in root or 14 not in root:
        raise ValueError('Missing RT_ICON or RT_GROUP_ICON')
    icons, groups = children(root[3]), children(root[14])
    group = payload(groups[min(groups)])
    reserved, kind, count = struct.unpack_from('<HHH', ico)
    if (reserved, kind) != (0, 1) or group[:6] != ico[:6]:
        raise ValueError('Icon group does not match the source ICO')
    sizes = []
    for i in range(count):
        source = 6 + i * 16
        target = 6 + i * 14
        if group[target:target + 12] != ico[source:source + 12]:
            raise ValueError(f'Icon entry {i} differs from the source ICO')
        length, start = struct.unpack_from('<II', ico, source + 8)
        resource_id = struct.unpack_from('<H', group, target + 12)[0]
        if payload(icons[resource_id]) != ico[start:start + length]:
            raise ValueError(f'Icon image {i} differs from the source ICO')
        sizes.append(ico[source] or 256)
    return sizes


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('executable', type=Path)
    parser.add_argument('--icon', type=Path, default=Path(__file__).resolve().parents[1] / 'assets/windows/netmerge.ico')
    args = parser.parse_args()
    try:
        sizes = verify(args.executable, args.icon)
    except (ValueError, KeyError, IndexError, struct.error) as error:
        parser.exit(1, f'{args.executable}: {error}\n')
    print(f'{args.executable}: NetMerge icon verified at {", ".join(map(str, sizes))} px')
