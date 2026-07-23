/** Collect File objects (with relative paths) from a folder drag or input. */

export type NamedFile = { file: File; relativePath?: string };

export async function filesFromDataTransfer(
  dt: DataTransfer,
): Promise<NamedFile[]> {
  const items = dt.items;
  if (!items?.length) {
    return [...dt.files].map((file) => ({
      file,
      relativePath: file.webkitRelativePath || undefined,
    }));
  }

  const entries: FileSystemEntry[] = [];
  for (let i = 0; i < items.length; i++) {
    const entry = items[i].webkitGetAsEntry?.();
    if (entry) entries.push(entry);
  }
  if (entries.length === 0) {
    return [...dt.files].map((file) => ({
      file,
      relativePath: file.webkitRelativePath || undefined,
    }));
  }

  const out: NamedFile[] = [];
  for (const entry of entries) {
    await walkEntry(entry, "", out);
  }
  return out;
}

async function walkEntry(
  entry: FileSystemEntry,
  prefix: string,
  out: NamedFile[],
): Promise<void> {
  if (entry.isFile) {
    const file = await new Promise<File>((resolve, reject) => {
      (entry as FileSystemFileEntry).file(resolve, reject);
    });
    const relativePath = prefix ? `${prefix}/${file.name}` : file.name;
    out.push({ file, relativePath });
    return;
  }
  if (entry.isDirectory) {
    const reader = (entry as FileSystemDirectoryEntry).createReader();
    const children = await readAllEntries(reader);
    const nextPrefix = prefix ? `${prefix}/${entry.name}` : entry.name;
    for (const child of children) {
      await walkEntry(child, nextPrefix, out);
    }
  }
}

function readAllEntries(
  reader: FileSystemDirectoryReader,
): Promise<FileSystemEntry[]> {
  return new Promise((resolve, reject) => {
    const all: FileSystemEntry[] = [];
    const read = () => {
      reader.readEntries((batch) => {
        if (batch.length === 0) {
          resolve(all);
          return;
        }
        all.push(...batch);
        read();
      }, reject);
    };
    read();
  });
}

export function namedFilesFromFileList(list: FileList | File[]): NamedFile[] {
  return [...list].map((file) => ({
    file,
    relativePath: file.webkitRelativePath || undefined,
  }));
}
