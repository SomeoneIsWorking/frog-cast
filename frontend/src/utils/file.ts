export function isAcceptedFile(
  filePath: string | undefined,
  acceptedExtensions: string[]
) {
  if (!acceptedExtensions || acceptedExtensions.length === 0) return true;
  const fileLower = (filePath || "").toLowerCase();
  if (!fileLower.includes(".")) return false;
  const ext = fileLower.split(".").pop() || "";
  return acceptedExtensions.some((accepted) => accepted.toLowerCase() === ext);
}

// isWebUrl reports whether media is a web page or stream URL rather than a
// local file.
export function isWebUrl(path: string): boolean {
  const lower = path.toLowerCase();
  return lower.startsWith("http://") || lower.startsWith("https://");
}

export function isAcceptedFileWithHttp(
  filePath: string,
  acceptedExtensions: string[]
) {
  if (isWebUrl(filePath)) {
    return true; // Accept all URLs
  }
  return isAcceptedFile(filePath, acceptedExtensions);
}
