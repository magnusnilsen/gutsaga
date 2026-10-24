local src = debug.getinfo(1, "S").source
local dir = src:match("^@(.*)[/\\][^/\\]+$") or "."
local M = dofile(dir .. "/GutSaga/GutSagaObjectives.lua")

local function assertTrue(cond, msg)
    if not cond then
        error(msg or "assertion failed", 2)
    end
end

local objectives = {
    { id = "place", type = "location", text = "Goldshire", map_id = 37, x = 0.438, y = 0.658, radius = 0.05 },
    { id = "talk", type = "interact", text = "Farley", npc = "Innkeeper Farley" },
    { id = "water", type = "collect", text = "Vann", item_id = 159, count = 1 },
}

local steps, done = M.evaluate(objectives, { map_id = 37, x = 0.44, y = 0.66 }, {})
assertTrue(steps.place.done, "location should match")

steps, done = M.evaluate(objectives, { map_id = 1429, x = 0.44, y = 0.66 }, {})
assertTrue(steps.place.done, "classic era map id still matches the same coordinates")

steps, done = M.evaluate(objectives, { map_id = 1429, x = 44, y = 66 }, {})
assertTrue(steps.place.done, "percent coordinates match")
assertTrue(not steps.talk.done, "talk is still locked until we pass saved done")
assertTrue(not done, "not finished")

steps, done = M.evaluate(objectives, { npc = "Innkeeper Farley", items = { [159] = 5 } }, {})
assertTrue(not steps.talk.done and not steps.water.done, "later steps stay locked")

steps, done = M.evaluate(objectives, {
    map_id = 37, x = 0.90, y = 0.10, npc = "Innkeeper Farley", items = { [159] = 5 },
}, { place = { done = true } })
assertTrue(steps.place.done, "stays done away from the inn")
assertTrue(steps.talk.done and steps.water.done and done, "later steps complete in the same pass")

steps, done = M.evaluate(objectives, { items = { ["159"] = 1 } }, {
    place = { done = true }, talk = { done = true },
})
assertTrue(steps.water.done and done, "all done")

steps, done = M.evaluate({
    { id = "kobold", type = "kill", npc = "Kobold Vermin", count = 2 },
}, { kill_name = "Kobold Vermin", kill_secret = true }, {})
assertTrue(steps.kobold.count == 0 and not steps.kobold.done, "secret kill ignored")

steps, done = M.evaluate({
    { id = "kobold", type = "kill", npc = "Kobold Vermin", count = 2 },
}, { kill_name = "kobold vermin" }, { kobold = { count = 1 } })
assertTrue(steps.kobold.count == 2 and steps.kobold.done, "kill increments")

steps, done = M.evaluate({}, {}, {})
assertTrue(not done, "empty quest does not complete")

steps, done = M.evaluate({
    { id = "obj", type = "interact", object = "Mailbox" },
}, { object = "Mailbox" }, {})
assertTrue(steps.obj.done and done, "object interact")
steps = M.evaluate({
    { id = "obj", type = "interact", object = "Mailbox" },
}, { npc = "Mailbox" }, {})
assertTrue(steps.obj.done, "gossip name counts as object")

steps, done = M.evaluate(objectives, { object = "Innkeeper Farley" }, { place = { done = true } })
assertTrue(steps.talk.done, "current target counts as the npc")

-- Any-order: later goals can complete without earlier ones.
steps, done = M.evaluate(objectives, { npc = "Innkeeper Farley", items = { [159] = 1 } }, {}, false)
assertTrue(not steps.place.done, "location still open")
assertTrue(steps.talk.done and steps.water.done, "interact and collect without place")
assertTrue(not done, "saga not finished until every goal is done")

steps, done = M.evaluate({
    { id = "place", type = "location", text = "Inn", subzone = "Goldshire|Lion's Pride Inn", map_id = 37, x = 0.438, y = 0.658, radius = 0.05 },
}, { minimap = "Lion's Pride Inn" }, {})
assertTrue(steps.place.done, "minimap label completes the inn when subzone text is empty")

steps, done = M.evaluate({
    { id = "place", type = "location", text = "Inn", subzone = "Goldshire|Lion's Pride Inn", map_id = 37, x = 0.438, y = 0.658, radius = 0.01 },
}, { subzone = "Goldshire", map_id = 999, x = 0.1, y = 0.1 }, {})
assertTrue(steps.place.done, "goldshire subzone completes just outside the inn")

steps, done = M.evaluate({
    { id = "place", type = "location", text = "Stå i eller rett utenfor Lion's Pride Inn i Goldshire", map_id = 37, x = 0.9, y = 0.9, radius = 0.01 },
}, { minimap = "Lion's Pride Inn" }, {})
assertTrue(steps.place.done, "objective wording matches the minimap when subzone is missing")

local night = { time = { ["local"] = { hour = 23, min = 15, unix = 1760000000 } } }
local day = { time = { ["local"] = { hour = 12, min = 0, unix = 1760000000 } } }
steps, done = M.evaluate({
    { id = "salute", type = "emote", text = "Hils", emote = "/salute", npc = "Stormwind Guard",
      map_id = 84, x = 0.50, y = 0.50, radius = 0.05,
      when = { clock = "local", start = "22:00", ["end"] = "02:00" } },
}, {
    emote = "SALUTE", emote_target = "Stormwind Guard",
    emote_map_id = 84, emote_x = 0.51, emote_y = 0.50,
    time = night.time,
}, {})
assertTrue(steps.salute.done and done, "emote at the guard during the window")

steps, done = M.evaluate({
    { id = "salute", type = "emote", text = "Hils", emote = "SALUTE", npc = "Stormwind Guard" },
}, { emote = "salute", emote_target = "Innkeeper Farley", time = night.time }, {})
assertTrue(not steps.salute.done, "wrong emote target")

steps, done = M.evaluate({
    { id = "salute", type = "emote", text = "Hils", emote = "WAVE",
      when = { start = "22:00", ["end"] = "02:00" } },
}, { emote = "WAVE", time = day.time }, {})
assertTrue(not steps.salute.done, "daytime emote stays open")

steps, done = M.evaluate({
    { id = "both", type = "all_of", text = "Sted og vann", objectives = {
        { id = "inn", type = "location", text = "Goldshire", zone = "Elwynn Forest" },
        { id = "water", type = "collect", text = "Vann", item_id = 159, count = 1 },
    } },
}, { zone = "Elwynn Forest", items = { [159] = 1 } }, {}, false)
assertTrue(steps.both.done and steps.inn.done and steps.water.done and done, "all_of is true together")

steps, done = M.evaluate({
    { id = "both", type = "all_of", text = "Sted og vann", objectives = {
        { id = "inn", type = "location", text = "Goldshire", zone = "Elwynn Forest" },
        { id = "water", type = "collect", text = "Vann", item_id = 159, count = 1 },
    } },
}, { zone = "Stormwind City", items = { [159] = 1 } }, { inn = { done = true } }, false)
assertTrue(not steps.both.done and not steps.inn.done, "nested location is live, not sticky")

steps, done = M.evaluate({
    { id = "either", type = "any_of", text = "Ett av dem", objectives = {
        { id = "talk", type = "interact", text = "Snakk", npc = "Farley" },
        { id = "bow", type = "emote", text = "Bukk", emote = "BOW" },
    } },
}, { emote = "BOW" }, {}, false)
assertTrue(steps.either.done and steps.bow.done and not steps.talk.done and done, "any_of emote")

steps, done = M.evaluate({
    { id = "stay", type = "location", text = "Vakt", zone = "Elwynn Forest",
      when = { clock = "realm", starts_at = "2026-10-24T20:00:00Z", ends_at = "2026-10-25T04:00:00Z" } },
}, { zone = "Elwynn Forest", time = { realm = { hour = 21, min = 0, unix = 1792875600 } } }, {}, false)
assertTrue(steps.stay.done, "realm date range includes this instant")

steps, done = M.evaluate({
    { id = "stay", type = "location", text = "Vakt", zone = "Elwynn Forest" },
}, { zone = "Elwynn Forest", time = { game = { hour = 10, min = 0 } } }, {}, false, { clock = "game", start = "22:00", ["end"] = "05:00" })
assertTrue(not steps.stay.done and not done, "saga game-time window blocks completion")

steps, done = M.evaluate({
    { id = "stay", type = "location", text = "Vakt", zone = "Elwynn Forest" },
}, { zone = "Elwynn Forest", time = { game = { hour = 23, min = 0 } } }, {}, false, { clock = "game", start = "22:00", ["end"] = "05:00" })
assertTrue(steps.stay.done and done, "saga window open")

local group = {
    instance = "The Deadmines",
    zone = "The Deadmines",
    members = {
        { name = "Magni", level = 18 },
        { name = "Mira", level = 17 },
        { name = "Pip", level = 12 },
        { name = "Ada", level = 20 },
    },
}
assertTrue(not M.groupOK({ size = 4, min_level = 17, instance = "Deadmines" }, group), "Pip is below 17 so only 3 count")
assertTrue(M.groupOK({ size = 3, min_level = 17, instance = "The Deadmines|Deadmines" }, group), "three at 17 in the instance")
assertTrue(not M.groupOK({ size = 3, min_level = 17, instance = "Stormwind Stockade" }, group), "wrong instance")
assertTrue(M.groupOK(nil, {}), "no group rule")

print("objectives ok")
