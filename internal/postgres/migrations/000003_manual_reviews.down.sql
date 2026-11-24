UPDATE sim_orders AS orders
   SET order_status = reviews.previous_status
  FROM sim_manual_reviews AS reviews
 WHERE orders.id = reviews.order_id AND orders.order_status = 'manual_review';

DROP TABLE sim_manual_reviews;
