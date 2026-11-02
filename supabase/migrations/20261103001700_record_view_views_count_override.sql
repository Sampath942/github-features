CREATE OR REPLACE FUNCTION public.record_view(
    p_username text,
    p_initial_views bigint DEFAULT NULL
) RETURNS SETOF public.view_count
    LANGUAGE sql
    AS $$
    INSERT INTO public.view_count AS vc (
        username,
        views_count
    )
    VALUES (
        p_username,
        COALESCE(p_initial_views, 1)
    )
    ON CONFLICT (username)
    DO UPDATE SET
        views_count = COALESCE(p_initial_views, vc.views_count + 1)
    RETURNING vc.*;
$$;
