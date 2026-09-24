import './style.css';
import { ScanPlugins, UpdatePlugin } from '../wailsjs/go/main/App';

const app = document.getElementById('app');

app.innerHTML = `
  <div class="header">
    <div>
      <h1>TotalPlug</h1>
      <div class="status-bar" id="status">Нажмите «Сканировать», чтобы проверить плагины...</div>
    </div>
    <div class="actions">
      <button id="btn-scan">🔍 Сканировать плагины</button>
      <button id="btn-refresh" style="background:#555">🔄 Обновить каталог</button>
    </div>
  </div>
  <div class="table-container">
    <table>
      <thead>
        <tr>
          <th>Тип</th>
          <th>Имя плагина</th>
          <th>Установлена</th>
          <th>Источник</th>
          <th>В каталоге</th>
          <th>Статус</th>
          <th>Действие</th>
        </tr>
      </thead>
      <tbody id="plugins-body">
        <tr><td colspan="7" style="text-align:center; color:#777; padding: 24px;">Плагины не загружены</td></tr>
      </tbody>
    </table>
  </div>
`;

const statusEl = document.getElementById('status');
const tbody = document.getElementById('plugins-body');
const btnScan = document.getElementById('btn-scan');
const btnRefresh = document.getElementById('btn-refresh');

let currentPlugins = [];

function getStatusBadge(status) {
    switch (status) {
        case 'up_to_date':
            return '<span class="badge badge-up">✓ Актуален</span>';
        case 'update_avail':
            return '<span class="badge badge-update">🚀 Обновление!</span>';
        case 'not_found':
            return '<span class="badge badge-err">✗ Отсутствует</span>';
        default:
            return '<span class="badge badge-muted">⚠️ Неизвестно</span>';
    }
}

async function runScan(forceRefresh = false) {
    try {
        btnScan.disabled = true;
        btnRefresh.disabled = true;
        statusEl.innerText = forceRefresh ? "Загрузка свежего каталога с GitHub..." : "Сканирование wincmd.ini и PE-ресурсов...";

        currentPlugins = await ScanPlugins(forceRefresh);

        if (!currentPlugins || currentPlugins.length === 0) {
            tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; padding:20px;">Плагины не найдены в wincmd.ini</td></tr>`;
            statusEl.innerText = "Сканирование завершено: плагины не обнаружены.";
            return;
        }

        renderTable(currentPlugins);
        statusEl.innerText = `Найдено плагинов: ${currentPlugins.length}`;
    } catch (err) {
        statusEl.innerText = `Ошибка: ${err}`;
        console.error(err);
    } finally {
        btnScan.disabled = false;
        btnRefresh.disabled = false;
    }
}

function renderTable(plugins) {
    tbody.innerHTML = plugins.map((p, idx) => {
        const canUpdate = p.status === 'update_avail' && p.download_url;
        return `
            <tr>
                <td><strong>${p.type}</strong></td>
                <td>${p.name}</td>
                <td><code>${p.installed_version}</code></td>
                <td><small style="color:#aaa">${p.confidence}</small></td>
                <td><code>${p.latest_version || '-'}</code></td>
                <td>${getStatusBadge(p.status)}</td>
                <td>
                    ${canUpdate ? `<button class="btn-update" onclick="handleUpdate(${idx})">Обновить</button>` : '-'}
                </td>
            </tr>
        `;
    }).join('');
}

window.handleUpdate = async function(index) {
    const plugin = currentPlugins[index];
    if (!plugin) return;

    if (!confirm(`Обновить плагин ${plugin.name} до версии ${plugin.latest_version}?`)) {
        return;
    }

    try {
        statusEl.innerText = `Обновление ${plugin.name}...`;
        await UpdatePlugin(plugin);
        alert(`Плагин ${plugin.name} успешно обновлен!`);
        runScan(false);
    } catch (err) {
        alert(`Ошибка обновления: ${err}`);
        statusEl.innerText = `Ошибка при обновлении ${plugin.name}`;
    }
};

btnScan.addEventListener('click', () => runScan(false));
btnRefresh.addEventListener('click', () => runScan(true));

// Автозапуск сканирования при старте окна
runScan(false);