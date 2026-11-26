extends SceneTree
var frame := 0
var samples: Array[float] = []
var previous := 0
var position_x := 0.0
var rectangle: ColorRect
var context: Dictionary
var delay_ms := 0
var draws := 0.0
var memory := 0.0
func _initialize():
    var file = FileAccess.open(OS.get_cmdline_user_args()[0], FileAccess.READ)
    context = JSON.parse_string(file.get_as_text())
    delay_ms = int(FileAccess.get_file_as_string(context.root.path_join("candidate/delay.txt")))
    rectangle = ColorRect.new()
    rectangle.color = Color(0.1, 0.6, 0.9)
    rectangle.size = Vector2(40, 40)
    root.add_child(rectangle)
    previous = Time.get_ticks_usec()
func _process(delta):
    if delay_ms > 0:
        OS.delay_msec(delay_ms)
    position_x += 1.0
    rectangle.position.x = fmod(position_x, 250.0)
    frame += 1
    var current = Time.get_ticks_usec()
    if frame > 30:
        samples.append(float(current - previous) / 1000.0)
        draws = max(draws, Performance.get_monitor(Performance.RENDER_TOTAL_DRAW_CALLS_IN_FRAME))
        memory = max(memory, Performance.get_monitor(Performance.MEMORY_STATIC))
    previous = current
    if frame >= 100:
        samples.sort()
        var checks = {"renderer-active": DisplayServer.get_name() != "headless" and draws > 0, "workload-complete": samples.size() == 70, "moves-right": position_x == 100.0}
        var report = {"checks":checks,"metrics":{"frame_p95_ms":samples[int(samples.size()*0.95)],"draw_calls":draws,"memory_bytes":memory}}
        root.get_texture().get_image().save_png(context.evidence_dir.path_join("game.png"))
        print("RECIPE_REPORT:" + JSON.stringify(report))
        quit()
    return false
