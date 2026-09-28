import client from './client'

export interface UploadedImage {
  url: string
  width: number
  height: number
  size: number
}

// The two folders the server will write into, each behind the permission that
// owns it: a product's cover, or a site image such as the shop Logo.
export type UploadScope = 'products' | 'site'

// The file's own name never reaches the server: the address that comes back is
// generated there, from what the bytes actually are.
export const uploadImage = async (file: File, scope: UploadScope = 'products'): Promise<UploadedImage> => {
  const form = new FormData()
  form.append('image', file)
  // The shared client defaults to `Content-Type: application/json`, and axios
  // honours that for a FormData body by stringifying it into JSON — the server
  // then has no multipart to read. Naming multipart/form-data here is what keeps
  // the body a real form; the adapter clears the value again for a FormData body,
  // so the browser is still the one that writes the boundary.
  const { data } = await client.post<UploadedImage>(`/admin/uploads/${scope}`, form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data
}
