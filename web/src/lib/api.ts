export interface UploadResult {
  fileId: string;
  fileName: string;
  fileSize: number;
  url: string;
  longURL: string;
  downloadURL: string;
  longDownloadURL: string;
}

/** API origin for server-side fetches (inside Docker: http://api:8080). */
export const API_URL = process.env.API_URL ?? "http://localhost:8080";
