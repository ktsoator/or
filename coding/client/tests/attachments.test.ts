import { describe, expect, test } from 'bun:test'
import {
  formatFileSize,
  isSupportedTextFile,
  maxTextFileBytes,
  maxTextFiles,
  validateTextFiles,
} from '../src/attachments'

describe('text attachments', () => {
  test('recognizes source files even when the browser omits a MIME type', () => {
    expect(isSupportedTextFile('main.go', '')).toBeTrue()
    expect(isSupportedTextFile('Dockerfile', '')).toBeTrue()
    expect(isSupportedTextFile('notes.custom', 'text/plain')).toBeTrue()
    expect(isSupportedTextFile('archive.zip', 'application/zip')).toBeFalse()
  })

  test('enforces count, per-file, and aggregate limits', () => {
    expect(
      validateTextFiles(
        Array.from({ length: maxTextFiles }, () => ({ size: 1 })),
        [{ name: 'next.txt', type: 'text/plain', size: 1 }],
      ),
    ).toBe('count')
    expect(
      validateTextFiles([], [
        { name: 'large.txt', type: 'text/plain', size: maxTextFileBytes + 1 },
      ]),
    ).toBe('file_size')
    expect(
      validateTextFiles(
        Array.from({ length: 4 }, () => ({ size: maxTextFileBytes })),
        [{ name: 'more.txt', type: 'text/plain', size: 1 }],
      ),
    ).toBe('total_size')
  })

  test('formats compact file sizes for attachment rows', () => {
    expect(formatFileSize(12)).toBe('12 B')
    expect(formatFileSize(1536)).toBe('2 KB')
    expect(formatFileSize(1 << 20)).toBe('1.0 MB')
  })
})
