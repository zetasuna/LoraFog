#!/bin/sh

# --- Cài đặt udev rules ---
if [ -f ./99-lora.rules ]; then
  sudo cp ./99-lora.rules /etc/udev/rules.d/
  sudo udevadm control --reload-rules
  sudo udevadm trigger
else
  echo "⚠️  Không tìm thấy file ./99-lora.rules — bỏ qua bước copy udev rules."
fi

# --- Tạo thư mục ~/.local/bin nếu chưa tồn tại ---
mkdir -p "$HOME/.local/bin"

# --- Copy service vào ~/.local/bin nếu file tồn tại ---
if [ -f ./lora.service ]; then
  cp ./lora.service "$HOME/.local/bin"
else
  echo "⚠️  Không tìm thấy file ./lora.service — bỏ qua bước copy."
fi

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
