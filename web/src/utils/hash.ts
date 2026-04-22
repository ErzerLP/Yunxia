import SparkMD5 from 'spark-md5'

export function computeFileHash(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunkSize = 5 * 1024 * 1024
    const chunks = Math.ceil(file.size / chunkSize)
    const spark = new SparkMD5.ArrayBuffer()
    const reader = new FileReader()
    let currentChunk = 0

    reader.onload = (e) => {
      const result = e.target?.result
      if (result instanceof ArrayBuffer) {
        spark.append(result)
      }
      currentChunk++
      if (currentChunk < chunks) {
        loadNext()
      } else {
        resolve(spark.end())
      }
    }

    reader.onerror = (e) => {
      reject(e)
    }

    function loadNext() {
      const start = currentChunk * chunkSize
      const end = Math.min(start + chunkSize, file.size)
      reader.readAsArrayBuffer(file.slice(start, end))
    }

    loadNext()
  })
}
