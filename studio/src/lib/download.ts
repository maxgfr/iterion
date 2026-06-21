// downloadBlob triggers a browser file download for a Blob via a
// temporary object URL + hidden anchor, then revokes the URL after a
// short delay. Centralized so the revoke-timing wisdom lives in ONE
// place: revoking synchronously (or at 0ms) can cancel the download in
// Firefox/Safari/WebKit before the browser has read the blob, producing
// "blob URL not found" failures. The anchor is appended to the DOM
// before click() because some browsers ignore .click() on a detached
// anchor.
//
// Browser-only. Callers that also support the desktop bridge
// (desktop.saveBinaryFile / saveTextFile) must branch on isDesktop()
// before calling this.
export function downloadBlob(
  blob: Blob,
  filename: string,
  opts?: { revokeMs?: number },
): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.style.display = "none";
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), opts?.revokeMs ?? 1000);
}
