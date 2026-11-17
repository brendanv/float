// downloadBlob saves `data` (a string, BufferSource or Blob part) as
// `filename` through a temporary link. The link is attached to the document
// for the click (some browsers ignore clicks on detached links), and the
// object URL is revoked a tick later so the download has started.
export function downloadBlob(data, filename, type = "application/octet-stream") {
  const url = URL.createObjectURL(new Blob([data], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
