import { describe, expect, test } from 'bun:test'
import {
  formatFileSize,
  isSupportedTextFile,
  maxImageBytes,
  maxImages,
  maxImagesBytes,
  maxTextFileBytes,
  maxTextFiles,
  validateImageFiles,
  validateTextFiles,
} from '../src/shared/attachments'

describe('image attachments', () => {
  test('accepts the supported image MIME types', () => {
    for (const type of ['image/gif', 'image/jpeg', 'image/png', 'image/webp']) {
      expect(validateImageFiles([], [{ type, size: 1 }])).toBeUndefined()
    }
    expect(validateImageFiles([], [{ type: 'image/svg+xml', size: 1 }])).toBe('type')
  })

  test('enforces count, per-file, and aggregate limits', () => {
    expect(
      validateImageFiles(
        Array.from({ length: maxImages }, () => ({ size: 1 })),
        [{ type: 'image/png', size: 1 }],
      ),
    ).toBe('count')
    expect(
      validateImageFiles([], [{ type: 'image/png', size: maxImageBytes + 1 }]),
    ).toBe('file_size')
    expect(
      validateImageFiles(
        [{ size: maxImagesBytes }],
        [{ type: 'image/png', size: 1 }],
      ),
    ).toBe('total_size')
  })
})

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
