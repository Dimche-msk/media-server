// Форматирование размеров в человекочитаемый вид.

export function formatSize(bytes) {
  const n = Number(bytes) || 0;
  if (n <= 0) return '';
  const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
  let i = 0;
  let v = n / 1024;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return i === 0 ? `${n} Б` : `${v.toFixed(1)} ${units[i]}`;
}
