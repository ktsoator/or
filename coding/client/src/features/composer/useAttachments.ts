import { useEffect, useRef, useState } from 'react'
import {
  maxImages,
  maxTextFiles,
  readImage,
  readTextFile,
  validateImageFiles,
  validateTextFiles,
} from '@/shared/attachments'
import { useI18n } from '@/i18n'
import type { PendingFile, PendingImage } from '@/types'

export function useComposerAttachments(supportsImages: boolean) {
  const { t } = useI18n()
  const imageFileRef = useRef<HTMLInputElement>(null)
  const textFileRef = useRef<HTMLInputElement>(null)
  const [images, setImages] = useState<PendingImage[]>([])
  const [files, setFiles] = useState<PendingFile[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    if (supportsImages) setError('')
  }, [supportsImages])

  const addImages = async (selectedFiles: FileList | null) => {
    if (!selectedFiles || selectedFiles.length === 0 || !supportsImages) return
    setError('')
    const selected = Array.from(selectedFiles)
    const validation = validateImageFiles(images, selected)
    if (validation) {
      const messages = {
        count: t('composer.maxImages', { count: maxImages }),
        type: t('composer.imageTypes'),
        file_size: t('composer.imageTooLarge'),
        total_size: t('composer.imagesTooLarge'),
      }
      setError(messages[validation])
      return
    }
    try {
      const added = await Promise.all(selected.map(readImage))
      setImages((current) => [...current, ...added])
    } catch {
      setError(t('composer.couldNotReadImage'))
    }
  }

  const addTextFiles = async (selectedFiles: FileList | null) => {
    if (!selectedFiles || selectedFiles.length === 0) return
    setError('')
    const selected = Array.from(selectedFiles)
    const validation = validateTextFiles(files, selected)
    if (validation) {
      const messages = {
        count: t('composer.maxFiles', { count: maxTextFiles }),
        type: t('composer.fileTypes'),
        file_size: t('composer.fileTooLarge'),
        total_size: t('composer.filesTooLarge'),
      }
      setError(messages[validation])
      return
    }
    try {
      const added = await Promise.all(selected.map(readTextFile))
      setFiles((current) => [...current, ...added])
    } catch {
      setError(t('composer.fileNotText'))
    }
  }

  const removeImage = (id: string) => {
    setImages((current) => current.filter((image) => image.id !== id))
    setError('')
  }

  const removeFile = (id: string) => {
    setFiles((current) => current.filter((file) => file.id !== id))
    setError('')
  }

  const clear = () => {
    setImages([])
    setFiles([])
    setError('')
  }

  const reportUnsupportedImages = () => {
    setError(t('composer.modelNoImages'))
  }

  return {
    imageFileRef,
    textFileRef,
    images,
    files,
    error,
    imageLimitReached: images.length >= maxImages,
    fileLimitReached: files.length >= maxTextFiles,
    addImages,
    addTextFiles,
    removeImage,
    removeFile,
    clear,
    reportUnsupportedImages,
  }
}
