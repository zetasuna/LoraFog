#!/bin/sh

# --- Cài đặt udev rules ---
if [! -f /etc/udev/rules.d/99-lora.rules ]; then
  sudo cp ./99-lora.rules /etc/udev/rules.d/
  sudo udevadm control --reload-rules
  sudo udevadm trigger
else
  echo "⚠️  Tìm thấy file 99-lora.rules — bỏ qua bước copy udev rules."
fi

# --- Copy service vào ~/.local/bin nếu file tồn tại ---
mkdir -p "$HOME/.local/bin"
if [! -f "$HOME/.config/systemd/user/lora.service" ]; then
  cp ./lora.service "$HOME/.config/systemd/user"
else
  echo "⚠️  Tìm thấy file lora.service — bỏ qua bước copy."
fi

# --- Tạo thư mục ~/.local/bin nếu chưa tồn tại ---
mkdir -p "$HOME/.local/bin"

# --- Thêm ~/.local/bin vào PATH nếu chưa có ---
case ":$PATH:" in
*:"$HOME/.local/bin":*)
  # đã có, không làm gì
  ;;
*)
  echo 'export PATH="$HOME/.local/bin:$PATH"' >>"$HOME/.bashrc"
  echo "Đã thêm ~/.local/bin vào PATH trong ~/.bashrc"
  ;;
esac

# --- Enable systemd user service ---
systemctl --user enable lora

echo "Hoàn tất"
