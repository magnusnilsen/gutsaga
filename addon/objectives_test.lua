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

print("objectives ok")
