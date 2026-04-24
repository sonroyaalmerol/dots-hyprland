<div align="center">
    <h1>【 Snry Shell 】</h1>
    <h3></h3>
</div>

<div align="center">

![](https://img.shields.io/github/last-commit/sonroyaalmerol/dots-hyprland?&style=for-the-badge&color=8ad7eb&logo=git&logoColor=D9E0EE&labelColor=1E202B)
![](https://img.shields.io/github/stars/sonroyaalmerol/dots-hyprland?style=for-the-badge&logo=andela&color=86dbd7&logoColor=D9E0EE&labelColor=1E202B)
![](https://img.shields.io/github/repo-size/sonroyaalmerol/dots-hyprland?color=86dbce&label=SIZE&logo=protondrive&style=for-the-badge&logoColor=D9E0EE&labelColor=1E202B)

</div>

<div align="center">
    <h2>• overview •</h2>
    <h3></h3>
</div>

<details> 
  <summary>What this is/isn't</summary>

- A Hyprland desktop shell configuration managed by Ansible
- NOT a system setup script: no graphics drivers, no zram setup, etc.

</details>

<details> 
  <summary>Notable features</summary>
     
  - **Overview**: Shows open apps with live previews
  - **AI**: Gemini, Ollama, and more
  - **QoL**: screen translation, anti-flashbang, Google Lens
  - **Material themes**: Choose your wallpaper, done, enjoy
  - **Transparent installation**: Every command is shown before it's run
</details>

<details> 
  <summary>Installation</summary>

- _If you're new to Linux and decide to use Hyprland, you're in for a tough ride._
- Install via AUR: `paru -S snry-shell-qs`, then run `snry-shell`
- Manual: Clone this repo and run `ansible-playbook setup.yml`
- **Keybinds**: Should be somewhat familiar to Windows or GNOME users. Important ones:
  - `Super`+`/` = keybind list
  - `Super`+`Enter` = terminal

</details>

<details>
  <summary>Software overview</summary>

| Software                                       | Purpose                                                                |
| ---------------------------------------------- | ---------------------------------------------------------------------- |
| [Hyprland](https://github.com/hyprwm/hyprland) | The compositor (manages and renders windows)                           |
| [Ansible](https://github.com/ansible/ansible)  | Configuration management and deployment                                |
| [Quickshell](https://quickshell.outfoxxed.me/) | A QtQuick-based widget system, used for the status bar, sidebars, etc. |
| Others                                         | See `data/deps-info.md` for complete dependency list                   |

</details>

<div align="center">
    <h2>• screenshots •</h2>
    <h3></h3>
</div>

<div align="center">
    <img src="assets/snry-shell.svg" alt="snry-shell logo" style="float:left; width:400;">
</div>

Widget system: Quickshell | Support: Yes

[Showcase video](https://www.youtube.com/watch?v=RPwovTInagE)

| AI, settings app                                                                                                                     | Some widgets                                                                                                                         |
| :----------------------------------------------------------------------------------------------------------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------- |
| <img width="1920" height="1080" alt="image" src="https://github.com/user-attachments/assets/5d4e7d07-d0b4-4406-a4c9-ed7ba90e3fe4" /> | <img width="1920" height="1080" alt="image" src="https://github.com/user-attachments/assets/6a32395f-9437-4192-8faf-2951a9e84cbe" /> |
| Window management                                                                                                                    | wow look its orange                                                                                                                  |
| <img width="1920" height="1080" alt="image" src="https://github.com/user-attachments/assets/c51bed8b-3670-4d4c-9074-873be224fb8e" /> | <img width="1920" height="1080" alt="image" src="https://github.com/user-attachments/assets/98703a66-0743-439f-a721-cef7afa6ab95" /> |

<div align="center">
    <h2>• thank you •</h2>
    <h3></h3>
</div>

- [@end-4](https://github.com/end-4) for the original "illogical impulse" / dots-hyprland project
- [@clsty](https://github.com/clsty) for making the dotfiles accessible by taking care of the install script and many other things
- [@midn8hustlr](https://github.com/midn8hustlr) for greatly improving the color generation system
- [@outfoxxed](https://github.com/outfoxxed/) for being extremely supportive in the Quickshell journey
- Quickshell: [Soramane](https://github.com/caelestia-dots/shell/), [FridayFaerie](https://github.com/FridayFaerie/quickshell), [nydragon](https://github.com/nydragon/nysh)
- AGS: [Aylur](https://github.com/Aylur/dotfiles/tree/ags-pre-ts), [kotontrion](https://github.com/kotontrion/dotfiles)
- EWW: [fufexan](https://github.com/fufexan/dotfiles)

<div align="center">
    <h2>• stonks •</h2>
    <h3></h3>
</div>

- Tentacle cat hub twinkle internet points

[![Stargazers over time](https://starchart.cc/sonroyaalmerol/dots-hyprland.svg?variant=adaptive)](https://starchart.cc/sonroyaalmerol/dots-hyprland)

---

<div align="center">
    <h2>• previous styles •</h2>
    <h3></h3>
</div>

- **Unsupported!**
- **Source**: snry-shell AGS in `ii-ags` branch, others in `archive` branch.
- List is in reverse chronological order

### snry-shell (AGS)

Widget system: AGS | Support: No

| AI                                                                                        | Common widgets                                                                                                 |
| :---------------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------- |
| ![image](https://github.com/user-attachments/assets/9d7af13f-89ef-470d-ba78-d2288b79cf60) | ![image](https://github.com/sonroyaalmerol/dots-hyprland/assets/97237370/406b72b6-fa38-4f0d-a6c4-4d7d5d5ddcb7) |
| Window management                                                                         | Weeb power                                                                                                     |
| ![image](https://github.com/user-attachments/assets/02983b9b-79ba-4c25-8717-90bef2357ae5) | ![image](https://github.com/user-attachments/assets/bbb332ec-962a-4e88-a95b-486d0bd8ce76)                      |

#### m3ww

Widget system: EWW | Support: No

<a href="https://streamable.com/85ch8x">
<img src="https://github.com/sonroyaalmerol/dots-hyprland/assets/97237370/09533e64-b6d7-47eb-a840-ee90c6776adf" alt="Material Eww!">
</a>

#### NovelKnock

Widget system: EWW | Support: No

<a href="https://streamable.com/7vo61k">
<img src="https://github.com/sonroyaalmerol/dots-hyprland/assets/97237370/42903d03-bf6f-49d4-be7f-dd77e6cb389d" alt="Desktop Preview">
</a>

#### Hybrid

Widget system: EWW | Support: No

<a href="https://streamable.com/4oogot">
<img src="https://github.com/sonroyaalmerol/dots-hyprland/assets/97237370/190deb1e-f6f5-46ce-8cf0-9b39944c079d" alt="click the circles!">
</a>

#### Windoes

Widget system: EWW | Support: No

<a href="https://streamable.com/5qx614">
<img src="https://github.com/sonroyaalmerol/dots-hyprland/assets/97237370/b15317b1-f295-49f5-b90c-fb6328b8d886" alt="Desktop Preview">
</a>

<div align="center">
    <h2>• inspirations/copying •</h2>
    <h3></h3>
</div>

- Inspiration: osu!lazer (Hybrid), Windows 11 (Windoes), AvdanOS (NovelKnock), Material Design 3 (m3ww & later)
- Copying: Absolutely, feel free. Just follow the license and it's all good
